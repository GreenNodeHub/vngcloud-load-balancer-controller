package controller

import (
	// "fmt"
	"time"

	"github.com/sirupsen/logrus"
	// "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	// "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	// "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

type kindStep string

const (
	createStep       kindStep = "create"
	updateStep       kindStep = "update"
	deleteStep       kindStep = "delete"
	updateStatusStep kindStep = "updateStatus"
)

type StepType struct {
	kindStep  kindStep             // create, update, delete
	name      string               // step name
	getObject func() client.Object // update objects such as ingress, service, endpoint,...
	expect    func()               // expect after update
}

type ObjectAndExpect[T kubernetesResource] struct {
	obj    T
	expect func()
}

type TestType[T kubernetesResource] struct {
	preTest           func()                      // prepare test
	name              string                      // test name
	generateDepends   func() []client.Object      // generate depend objects such as service, endpoint,...
	generateObj       func() []ObjectAndExpect[T] // generate object and expect
	expect            func()                      // expect after create all objects
	steps             []StepType                  // update and expect for each step
	expectAfterDelete func()                      // expect after delete all
	postTest          func()                      // clean up the preTest
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

	objs := tt.generateObj()
	Expect(objs).NotTo(BeNil())
	for _, obj := range objs {
		Expect(obj.obj).NotTo(BeNil())
		Expect(k8sClient.Create(ctx, obj.obj)).Should(Succeed())
		obj.expect()
	}

	if tt.expect != nil {
		tt.expect()
	}

	if tt.steps != nil {
		for _, step := range tt.steps {
			logrus.Infof("###### STEP: %s, kind: %s", step.name, step.kindStep)
			obj := step.getObject()
			Expect(obj).NotTo(BeNil())
			switch step.kindStep {
			case createStep:
				Expect(k8sClient.Create(ctx, obj)).Should(Succeed())
			case deleteStep:
				Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
			case updateStep:
				Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
			case updateStatusStep:
				Expect(k8sClient.Status().Update(ctx, obj)).Should(Succeed())
			default:
				logrus.Fatalf("Unknown step kind: %s of STEP %s", step.kindStep, step.name)
			}

			// expect load balancer attribute in the mock provider
			if step.expect != nil {
				step.expect()
			}
		}
	}

	// clean up, delete in reverse order, ignore error if object not found
	for i := len(objs) - 1; i >= 0; i-- {
		obj := objs[i].obj
		err := k8sClient.Delete(ctx, obj)
		Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())

		// wait for reconcile
		time.Sleep(timeWaitRecocile)

		// get will return error
		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}

	// expect after delete
	if tt.expectAfterDelete != nil {
		tt.expectAfterDelete()
	}

	for _, depend := range depends {
		err := k8sClient.Delete(ctx, depend)
		Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())

		// get will return error
		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(depend), depend)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}
	if tt.postTest != nil {
		tt.postTest()
	}
	printEndTest()
}

// // get load balancer by id in resource annotation
// func getLBByAnnotation[T kubernetesResource](k8sClient client.Client, obj T) *entity.LoadBalancer {
// 	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)).Should(Succeed())
// 	loadbalancerID := obj.GetAnnotations()[fmt.Sprintf("%s/%s", domain.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
// 	Expect(loadbalancerID).ShouldNot(BeEmpty())
// 	loadbalancer, err := mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
// 	Expect(err).ShouldNot(HaveOccurred())
// 	return loadbalancer
// }

// func getGLBByAnnotation[T kubernetesResource](k8sClient client.Client, obj T) *entity.GlobalLoadBalancer {
// 	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)).Should(Succeed())
// 	loadbalancerID := obj.GetAnnotations()[fmt.Sprintf("%s/%s", domain.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
// 	Expect(loadbalancerID).ShouldNot(BeEmpty())
// 	loadbalancer, err := mockProvider.GetGlobalLoadBalancerByID(ctx, loadbalancerID)
// 	Expect(err).ShouldNot(HaveOccurred())
// 	return loadbalancer
// }
