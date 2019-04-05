package manager

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha4"
	"github.com/weaveworks/eksctl/pkg/testutils/mockprovider"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ = Describe("StackCollection Tasks", func() {
	var (
		p   *mockprovider.MockProvider
		cfg *api.ClusterConfig

		stackManager *StackCollection
	)

	testAZs := []string{"us-west-2b", "us-west-2a", "us-west-2c"}

	newClusterConfig := func(clusterName string) *api.ClusterConfig {
		cfg := api.NewClusterConfig()
		*cfg.VPC.CIDR = api.DefaultCIDR()

		ng1 := cfg.NewNodeGroup()
		ng2 := cfg.NewNodeGroup()

		cfg.Metadata.Region = "us-west-2"
		cfg.Metadata.Name = clusterName
		cfg.AvailabilityZones = testAZs

		ng1.Name = "bar"
		ng1.InstanceType = "t2.medium"
		ng1.AMIFamily = "AmazonLinux2"
		ng2.Labels = map[string]string{"bar": "bar"}

		ng2.Name = "foo"
		ng2.InstanceType = "t2.medium"
		ng2.AMIFamily = "AmazonLinux2"
		ng2.Labels = map[string]string{"foo": "foo"}

		return cfg
	}

	Describe("TaskTree", func() {

		Context("With various sets of nested dummy tasks", func() {

			It("should have nice description", func() {
				{
					tasks := &TaskTree{Parallel: false}
					tasks.Append(&TaskTree{Parallel: false})
					Expect(tasks.Describe()).To(Equal("1 task: { no tasks }"))
					tasks.Sub = true
					tasks.DryRun = true
					tasks.Append(&TaskTree{Parallel: false, Sub: true})
					Expect(tasks.Describe()).To(Equal("(dry-run) 2 sequential sub-tasks: { no tasks, no tasks }"))
				}

				{
					tasks := &TaskTree{Parallel: false}
					subTask1 := &TaskTree{Parallel: false, Sub: true}
					subTask1.Append(&taskWithoutParams{
						info: "t1.1",
					})
					tasks.Append(subTask1)

					Expect(tasks.Describe()).To(Equal("1 task: { t1.1 }"))

					subTask2 := &TaskTree{Parallel: false, Sub: true}
					subTask2.Append(&taskWithoutParams{
						info: "t2.1",
					})
					subTask3 := &TaskTree{Parallel: true, Sub: true}
					subTask3.Append(&taskWithoutParams{
						info: "t3.1",
					})
					subTask3.Append(&taskWithoutParams{
						info: "t3.2",
					})
					tasks.Append(subTask2)
					subTask1.Append(subTask3)

					Expect(tasks.Describe()).To(Equal("2 sequential tasks: { 2 sequential sub-tasks: { t1.1, 2 parallel sub-tasks: { t3.1, t3.2 } }, t2.1 }"))
				}
			})

			It("should execute orderly", func() {
				{
					called11 := false

					tasks := &TaskTree{Parallel: false}
					subTask1 := &TaskTree{Parallel: false, Sub: true}
					subTask1.Append(&taskWithoutParams{
						info: "t1.1",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(100 * time.Millisecond)
								called11 = true
								errs <- nil
								close(errs)
							}()
							return nil
						},
					})
					tasks.Append(subTask1)

					called21 := false

					subTask2 := &TaskTree{Parallel: false, Sub: true}
					subTask2.Append(&taskWithoutParams{
						info: "t2.1",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(150 * time.Millisecond)
								called21 = true
								errs <- fmt.Errorf("t2.1 always fails")
								close(errs)
							}()
							return nil
						},
					})
					tasks.Append(subTask2)

					called31 := false
					called32 := false

					subTask3 := &TaskTree{Parallel: true, Sub: true}
					subTask3.Append(&taskWithoutParams{
						info: "t3.1",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(200 * time.Millisecond)
								called31 = true
								errs <- nil
								close(errs)
							}()
							return nil
						},
					})
					subTask3.Append(&taskWithoutParams{
						info: "t3.2",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(350 * time.Millisecond)
								called32 = true
								errs <- fmt.Errorf("t3.2 always fails")
								close(errs)
							}()
							return nil
						},
					})
					subTask1.Append(subTask3)

					Expect(tasks.Describe()).To(Equal("2 sequential tasks: { 2 sequential sub-tasks: { t1.1, 2 parallel sub-tasks: { t3.1, t3.2 } }, t2.1 }"))

					errs := tasks.DoAllSync()
					Expect(errs).To(HaveLen(2))
					Expect(errs[0].Error()).To(Equal("t3.2 always fails"))
					Expect(errs[1].Error()).To(Equal("t2.1 always fails"))

					Expect(called11).To(BeTrue())
					Expect(called21).To(BeTrue())
					Expect(called31).To(BeTrue())
					Expect(called32).To(BeTrue())
				}

				{
					tasks := &TaskTree{Parallel: false}
					Expect(tasks.DoAllSync()).To(HaveLen(0))
				}

				{
					tasks := &TaskTree{Parallel: false}
					tasks.Append(&TaskTree{Parallel: false})
					tasks.Append(&TaskTree{Parallel: true})
					Expect(tasks.DoAllSync()).To(HaveLen(0))
				}

				{
					tasks := &TaskTree{Parallel: true}

					tasks.Append(&taskWithoutParams{
						info: "t1.1",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(10 * time.Millisecond)
								errs <- nil
								close(errs)
							}()
							return nil
						},
					})

					tasks.Append(&taskWithoutParams{
						info: "t1.2",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(100 * time.Millisecond)
								errs <- fmt.Errorf("t1.2 always fails")
								close(errs)
							}()
							return nil
						},
					})

					tasks.Append(&taskWithoutParams{
						info: "t1.3",
						call: func(errs chan error) error {
							go func() {
								errs <- fmt.Errorf("t1.3 always fails")
								close(errs)
							}()
							return nil
						},
					})

					tasks.DryRun = true

					Expect(tasks.DoAllSync()).To(HaveLen(0))

					tasks.DryRun = false
					errs := tasks.DoAllSync()
					Expect(errs).To(HaveLen(2))
					Expect(errs[0].Error()).To(Equal("t1.3 always fails"))
					Expect(errs[1].Error()).To(Equal("t1.2 always fails"))
				}

				{
					tasks := &TaskTree{Parallel: false}

					tasks.Append(&taskWithoutParams{
						info: "t1.1",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(100 * time.Millisecond)
								errs <- fmt.Errorf("t1.1 always fails")
								close(errs)
							}()
							return nil
						},
					})

					tasks.Append(&taskWithoutParams{
						info: "t1.3",
						call: func(errs chan error) error {
							go func() {
								time.Sleep(150 * time.Millisecond)
								errs <- nil
								close(errs)
							}()
							return nil
						},
					})

					tasks.Append(&taskWithoutParams{
						info: "t1.3",
						call: func(errs chan error) error {
							go func() {
								errs <- fmt.Errorf("t1.3 always fails")
								close(errs)
							}()
							return nil
						},
					})

					tasks.DryRun = true

					Expect(tasks.DoAllSync()).To(HaveLen(0))

					tasks.DryRun = false
					errs := tasks.DoAllSync()
					Expect(errs).To(HaveLen(2))
					Expect(errs[0].Error()).To(Equal("t1.1 always fails"))
					Expect(errs[1].Error()).To(Equal("t1.3 always fails"))
				}

			})
		})

		Context("With real tasks", func() {

			BeforeEach(func() {

				p = mockprovider.NewMockProvider()

				cfg = newClusterConfig("test-cluster")

				stackManager = NewStackCollection(p, cfg)
			})

			It("should have nice description", func() {
				{
					tasks := stackManager.CreateTasksForNodeGroups(nil)
					Expect(tasks.Describe()).To(Equal(`2 parallel tasks: { create nodegroup "bar", create nodegroup "foo" }`))
				}
				{
					tasks := stackManager.CreateTasksForNodeGroups(sets.NewString("bar"))
					Expect(tasks.Describe()).To(Equal(`1 task: { create nodegroup "bar" }`))
				}
				{
					tasks := stackManager.CreateTasksForNodeGroups(sets.NewString("foo"))
					Expect(tasks.Describe()).To(Equal(`1 task: { create nodegroup "foo" }`))
				}
				{
					tasks := stackManager.CreateTasksForNodeGroups(sets.NewString())
					Expect(tasks.Describe()).To(Equal(`no tasks`))
				}
				{
					tasks := stackManager.CreateTasksForClusterWithNodeGroups(nil)
					Expect(tasks.Describe()).To(Equal(`2 sequential tasks: { create cluster control plane "test-cluster", 2 parallel sub-tasks: { create nodegroup "bar", create nodegroup "foo" } }`))
				}
				{
					tasks := stackManager.CreateTasksForClusterWithNodeGroups(sets.NewString("bar"))
					Expect(tasks.Describe()).To(Equal(`2 sequential tasks: { create cluster control plane "test-cluster", create nodegroup "bar" }`))
				}
				{
					tasks := stackManager.CreateTasksForClusterWithNodeGroups(sets.NewString())
					Expect(tasks.Describe()).To(Equal(`1 task: { create cluster control plane "test-cluster" }`))
				}
			})
		})

	})
})
