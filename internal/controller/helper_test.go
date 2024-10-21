package controller

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

type StepType struct {
	name          string                        // step name
	updateObjects func() []client.Object        // update objects such as ingress, service, endpoint,...
	expect        func(lb *entity.LoadBalancer) // expect after update
}

type TestType[T kubernetesResource] struct {
	preTest           func()                        // prepare test
	name              string                        // test name
	generateDepends   func() []client.Object        // generate depend objects such as service, endpoint,...
	generateObj       func() T                      // generate main object
	expect            func(lb *entity.LoadBalancer) // expect after create
	steps             []StepType                    // update and expect for each step
	expectAfterDelete func()                        // expect after clean up
	postTest          func()                        // expect after clean up
}

func RunMultiStepTest[T kubernetesResource](tt TestType[T]) {
	time.Sleep(2 * time.Second)
	logrus.Info("------------------- ", tt.name, " -------------------")
	if tt.preTest != nil {
		tt.preTest()
	}
	depends := tt.generateDepends()
	for _, depend := range depends {
		Expect(depend).NotTo(BeNil())
		Expect(k8sClient.Create(ctx, depend)).Should(Succeed())
	}

	obj := tt.generateObj()
	objName := obj.GetName()
	objNamespace := obj.GetNamespace()
	Expect(obj).NotTo(BeNil())
	Expect(k8sClient.Create(ctx, obj)).Should(Succeed())

	// get load balancer id in the annotation
	loadbalancerID := ""
	Eventually(func() bool {
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: objName, Namespace: objNamespace}, obj)).Should(Succeed())
		loadbalancerID = obj.GetAnnotations()[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
		return loadbalancerID != ""
	}, timeout, interval).Should(BeTrue())

	// expect load balancer attribute in the mock provider
	loadbalancer, err := mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
	Expect(err).ShouldNot(HaveOccurred())
	tt.expect(loadbalancer)

	if tt.steps != nil {
		for _, step := range tt.steps {
			logrus.Info("###### STEP: ", step.name)
			updateObjs := step.updateObjects()
			for _, obj := range updateObjs {
				Expect(obj).NotTo(BeNil())
				Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
			}

			// expect load balancer attribute in the mock provider
			step.expect(loadbalancer)
		}
	}

	// clean up
	Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: objName, Namespace: objNamespace}, obj)
		return err != nil
	}, 2*timeout, interval).Should(BeTrue())
	// _, err = mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
	// Expect(err).Should(HaveOccurred())

	// expect after delete
	if tt.expectAfterDelete != nil {
		tt.expectAfterDelete()
	}

	for _, depend := range depends {
		Expect(k8sClient.Delete(ctx, depend)).Should(Succeed())
		err := k8sClient.Get(ctx, client.ObjectKey{Name: depend.GetName(), Namespace: depend.GetNamespace()}, depend)
		Expect(err).Should(HaveOccurred())
	}
	if tt.postTest != nil {
		tt.postTest()
	}
	printEndTest()
}
