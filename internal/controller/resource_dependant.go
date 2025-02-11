package controller

import (
	// "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type kubernetesResource interface {
	metav1.Object
	runtime.Object
}
type ResourceDependant[T kubernetesResource] interface {
	// find all resources that depend on this resource such as Service, Endpoint. Endpoint is optional
	Set(service T, isAddEndpoint bool)
	// input is resource and return all resources that depend on this resource
	GetResourceNeedReconcile(kind, namespace, resource string) []reconcile.Request
	// clear all resources that depend on this resource
	Clear(resource T)
}

// --------------------------------------------------------------------------------------------
var _ ResourceDependant[*corev1.Service] = &serviceDependant{}

func NewServiceDependant(k8sClient client.Client) ResourceDependant[*corev1.Service] {
	return &serviceDependant{
		k8sClient:              k8sClient,
		serviceDependResources: make(map[string][]string),
	}
}

type serviceDependant struct {
	k8sClient              client.Client
	serviceDependResources map[string][]string
}

func (r *serviceDependant) Set(service *corev1.Service, isAddEndpoint bool) {
	r.Clear(service)
	namespace := service.Namespace
	name := service.Name
	key := namespace + "/" + name
	if isAddEndpoint {
		endpointKey := "endpoint/" + key
		r.serviceDependResources[key] = append(r.serviceDependResources[key], endpointKey)
	}

	// logrus.Infof("SetService: %v", r.serviceDependResources)
}

func (r *serviceDependant) GetResourceNeedReconcile(kind, namespace, resource string) []reconcile.Request {
	if kind != "endpoint" {
		return nil
	}
	result := []reconcile.Request{}
	key := kind + "/" + namespace + "/" + resource
	for k, v := range r.serviceDependResources {
		for _, d := range v {
			if d == key {
				namespace, name := revertKey(k)
				result = append(result, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Namespace: namespace,
						Name:      name,
					},
				})
			}
		}
	}
	return result
}

func (r *serviceDependant) Clear(service *corev1.Service) {
	key := service.Namespace + "/" + service.Name
	delete(r.serviceDependResources, key)
}

// --------------------------------------------------------------------------------------------

var _ ResourceDependant[*networkv1.Ingress] = &ingressDependant{}

func NewIngressDependant(k8sClient client.Client) ResourceDependant[*networkv1.Ingress] {
	return &ingressDependant{
		k8sClient:              k8sClient,
		ingressDependResources: make(map[string][]string),
	}
}

type ingressDependant struct {
	k8sClient              client.Client
	ingressDependResources map[string][]string
}

func (r *ingressDependant) Set(ingress *networkv1.Ingress, isAddEndpoint bool) {
	r.Clear(ingress)
	namespace := ingress.Namespace
	name := ingress.Name
	key := namespace + "/" + name
	if ingress.Spec.DefaultBackend != nil {
		serviceKey := "service/" + namespace + "/" + ingress.Spec.DefaultBackend.Service.Name
		r.ingressDependResources[key] = append(r.ingressDependResources[key], serviceKey)
		if isAddEndpoint {
			endpointKey := "endpoint/" + namespace + "/" + ingress.Spec.DefaultBackend.Service.Name
			r.ingressDependResources[key] = append(r.ingressDependResources[key], endpointKey)
		}
	}
	for _, rule := range ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			serviceKey := "service/" + namespace + "/" + path.Backend.Service.Name
			r.ingressDependResources[key] = append(r.ingressDependResources[key], serviceKey)
			if isAddEndpoint {
				endpointKey := "endpoint/" + namespace + "/" + path.Backend.Service.Name
				r.ingressDependResources[key] = append(r.ingressDependResources[key], endpointKey)
			}
		}
	}

	// get all secret name in ingress
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName != "" {
			secretKey := "secret/" + namespace + "/" + tls.SecretName
			r.ingressDependResources[key] = append(r.ingressDependResources[key], secretKey)
		}
	}
	// logrus.Infof("SetIngress: %v", r.ingressDependResources)
}

func (r *ingressDependant) GetResourceNeedReconcile(kind, namespace, resource string) []reconcile.Request {
	if kind != "endpoint" && kind != "service" && kind != "secret" {
		return nil
	}
	result := []reconcile.Request{}
	key := kind + "/" + namespace + "/" + resource
	for k, v := range r.ingressDependResources {
		for _, d := range v {
			if d == key {
				namespace, name := revertKey(k)
				result = append(result, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Namespace: namespace,
						Name:      name,
					},
				})
			}
		}
	}
	return result
}

func (r *ingressDependant) Clear(service *networkv1.Ingress) {
	key := service.Namespace + "/" + service.Name
	delete(r.ingressDependResources, key)
}
