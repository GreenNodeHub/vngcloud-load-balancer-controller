/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Ingress Controller", func() {
	Context("Wait 5 seconds before start test", func() {
		It("should be alright", func() {
			time.Sleep(5 * time.Second)
		})
	})

	Context("When update status", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest
			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			// update status
			ingress = &networkingv1.Ingress{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())

			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "10.0.0.1"}}
			Expect(k8sClient.Status().Update(ctx, ingress)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())
				return ingress != nil &&
					len(ingress.Status.LoadBalancer.Ingress) > 0 &&
					ingress.Status.LoadBalancer.Ingress[0].IP == "10.0.0.1"
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update status inside reconcile", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)

				// update status
				ingress := &networkingv1.Ingress{}
				k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)

				ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "10.0.0.1"}}
				k8sClient.Status().Update(ctx, ingress)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())
				return ingress != nil &&
					len(ingress.Status.LoadBalancer.Ingress) > 0 &&
					ingress.Status.LoadBalancer.Ingress[0].IP == "10.0.0.1"
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update annotaion", func() {
		It("should reconcile immediately", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			updateIngressAnnotation("test-ingress", "default", "test", "test")
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 3
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update annotation in whitelist annotation", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			updateIngressAnnotation("test-ingress", "default", "example.com/whitelist-annotation-1", "test")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(1))

			updateIngressAnnotation("test-ingress", "default", "example.com/blacklist-annotation-1", "test")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			updateIngressAnnotation("test-ingress", "default", "example.com/whitelist-annotation-1", "-")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			updateIngressAnnotation("test-ingress", "default", "example.com/blacklist-annotation-1", "-")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(3))

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 4
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When create ingress with specific annotation", func() {
		It("created load balancer shoud have specific attribute", func() {
			mockIngressReconciler.modeTest = false
			const (
				serviceName = "test-service-gogsf"
				ingressName = "test-ingress-gogsf"
				namespace   = "default"
			)

			// when create a service NodePort type, nothing should happen
			service := newServiceNodePortResource(serviceName, namespace)
			Expect(service).NotTo(BeNil())
			service.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// create ingress with specific annotation
			ingress := newIngressResource(ingressName, namespace)
			Expect(ingress).NotTo(BeNil())
			ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: serviceName,
					Port: networkingv1.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			ingress.Annotations = map[string]string{
				fmt.Sprintf("%s/%s", consts.INGRESS_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerName): "test-lb",
			}
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			// get load balancer id in the annotation
			loadbalancerID := ""
			Eventually(func() bool {
				getObject := &networkingv1.Ingress{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: namespace}, getObject)).Should(Succeed())
				loadbalancerID = getObject.Annotations[fmt.Sprintf("%s/%s", consts.INGRESS_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
				return loadbalancerID != ""
			}, timeout, interval).Should(BeTrue())

			// expect load balancer attribute in the mock provider
			loadbalancer, err := mockProvider.GetLoadBalancerByID(loadbalancerID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())
			Eventually(func() bool {
				loadbalancer, err = mockProvider.GetLoadBalancerByID(loadbalancerID)
				return err == nil && loadbalancer != nil && loadbalancer.DisplayStatus == consts.ACTIVE_LOADBALANCER_STATUS
			}, 2*timeout, interval).Should(BeTrue())
			Expect(loadbalancer.UUID).Should(Equal(loadbalancerID))
			Expect(loadbalancer.Name).Should(Equal("test-lb"))

			// clean up
			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				getObject := &networkingv1.Ingress{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: namespace}, getObject)
				return err != nil
			}, 2*timeout, interval).Should(BeTrue())
			_, err = mockProvider.GetLoadBalancerByID(loadbalancerID)
			Expect(err).Should(HaveOccurred())

			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				getObject := &corev1.Service{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, getObject)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	// Context("aaaaaaaaaaaaaaaa", func() {
	// 	It("aaaaaaaaaaaaaaaaaaaaaa", func() {
	// 	})
	// })

	// Context("aaaaaaaaaaaaaaaa", func() {
	// 	It("aaaaaaaaaaaaaaaaaaaaaa", func() {
	// 	})
	// })
})

func newIngressResource(name, namespace string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"annd2.space": "test",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: PointerOf("vngcloud"),
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "kubernetes",
					Port: networkingv1.ServiceBackendPort{
						Number: 443,
					},
				},
			},
		},
	}
}

func updateIngressAnnotation(name, namespace, key, value string) {
	ingress := &networkingv1.Ingress{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
	if value == "-" {
		delete(ingress.Annotations, key)
	} else {
		ingress.Annotations[key] = value
	}
	Expect(k8sClient.Update(ctx, ingress)).Should(Succeed())

	if value == "-" {
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
			return ingress != nil && ingress.Annotations[key] == ""
		}, timeout, interval).Should(BeTrue())
	} else {
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
			return ingress != nil && ingress.Annotations[key] == value
		}, timeout, interval).Should(BeTrue())
	}
}

func newServiceNodePortResource(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
}
