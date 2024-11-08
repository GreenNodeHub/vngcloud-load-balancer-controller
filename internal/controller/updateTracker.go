package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UpdateTrackerInterface interface {
	Start(context.Context)

	AddService(lbID, updateAt string, service *corev1.Service)
	RemoveService(lbID string, service *corev1.Service)
	GetServiceRequests() []reconcile.Request

	AddIngress(lbID, updateAt string, ingress *networkingv1.Ingress)
	RemoveIngress(lbID string, ingress *networkingv1.Ingress)
	GetIngressRequests() []reconcile.Request
}

type LoadBalancerWithTags struct {
	LoadBalancer *entityv2.LoadBalancer
	Tag          []*entityv2.Tag
}

var _ UpdateTrackerInterface = &UpdateTracker{}

type UpdateTracker struct {
	serviceTracker map[string]map[string]string
	ingressTracker map[string]map[string]string

	currentState []*entityv2.LoadBalancer
	provider     provider.Provider

	once sync.Once
	mu   sync.Mutex
}

func NewUpdateTracker(provider provider.Provider) *UpdateTracker {
	return &UpdateTracker{
		serviceTracker: make(map[string]map[string]string),
		ingressTracker: make(map[string]map[string]string),
		currentState:   make([]*entityv2.LoadBalancer, 0),
		provider:       provider,
	}
}

func (c *UpdateTracker) Start(ctx context.Context) {
	c.once.Do(func() {
		go func() {
			for {
				err := c.UpdateState(ctx)
				if err != nil {
					logrus.Errorf("Failed to update state: %v", err)
				}
				time.Sleep(60 * time.Second)
			}
		}()
	})
}

func (c *UpdateTracker) UpdateState(ctx context.Context) error {
	logrus.Debug("Updating state")
	c.mu.Lock()
	defer c.mu.Unlock()
	lbs, err := c.provider.ListLoadBalancers(ctx)
	if err != nil {
		return err
	}
	c.currentState = lbs.Items
	return nil
}

func (c *UpdateTracker) AddService(lbID, updateAt string, service *corev1.Service) {
	namespace, name := service.Namespace, service.Name
	if _, ok := c.serviceTracker[lbID]; !ok {
		c.serviceTracker[lbID] = make(map[string]string)
		c.serviceTracker[lbID][genKey(namespace, name)] = updateAt
	} else {
		if _, ok := c.serviceTracker[lbID][genKey(namespace, name)]; !ok {
			c.serviceTracker[lbID][genKey(namespace, name)] = updateAt
		} else {
			c.serviceTracker[lbID][genKey(namespace, name)] = updateAt
		}
	}
	c.UpdateState(context.Background())
}

func (c *UpdateTracker) AddIngress(lbID, updateAt string, ingress *networkingv1.Ingress) {
	namespace, name := ingress.Namespace, ingress.Name
	if _, ok := c.ingressTracker[lbID]; !ok {
		c.ingressTracker[lbID] = make(map[string]string)
		c.ingressTracker[lbID][genKey(namespace, name)] = updateAt
	} else {
		if _, ok := c.ingressTracker[lbID][genKey(namespace, name)]; !ok {
			c.ingressTracker[lbID][genKey(namespace, name)] = updateAt
		} else {
			c.ingressTracker[lbID][genKey(namespace, name)] = updateAt
		}
	}
	c.UpdateState(context.Background())
}

func (c *UpdateTracker) RemoveService(lbID string, service *corev1.Service) {
	if _, ok := c.serviceTracker[lbID]; ok {
		delete(c.serviceTracker[lbID], genKey(service.Namespace, service.Name))
		if len(c.serviceTracker[lbID]) == 0 {
			delete(c.serviceTracker, lbID)
		}
	}
}

func (c *UpdateTracker) RemoveIngress(lbID string, ingress *networkingv1.Ingress) {
	if _, ok := c.ingressTracker[lbID]; ok {
		delete(c.ingressTracker[lbID], genKey(ingress.Namespace, ingress.Name))
		if len(c.ingressTracker[lbID]) == 0 {
			delete(c.ingressTracker, lbID)
		}
	}
}

func (c *UpdateTracker) GetServiceRequests() []reconcile.Request {
	lbs := c.currentState

	isCheck := make(map[string]bool)
	lbIDs := make([]string, 0)
	for key := range c.serviceTracker {
		lbIDs = append(lbIDs, key)
		isCheck[key] = false
	}
	logrus.Infof("Watching these service loadbalancers: %s.", strings.Join(lbIDs, ", "))

	reapplyRequests := make([]reconcile.Request, 0)
	for _, lbwt := range lbs {
		lb := lbwt
		if _, ok := c.serviceTracker[lb.UUID]; ok {
			isCheck[lb.UUID] = true
			for key := range c.serviceTracker[lb.UUID] {
				if c.serviceTracker[lb.UUID][key] != lb.UpdatedAt {
					logrus.Infof("Loadbalancer %s has been updated, sync now.", lb.UUID)
					namespace, name := revertKey(key)
					reapplyRequests = append(reapplyRequests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespace,
							Name:      name,
						},
					})
					delete(c.serviceTracker[lb.UUID], key)
				}
			}
		}
	}
	for lbID, value := range isCheck {
		if !value {
			logrus.Infof("Loadbalancer %s has been deleted, sync now.", lbID)
			for key := range c.serviceTracker[lbID] {
				namespace, name := revertKey(key)
				reapplyRequests = append(reapplyRequests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				})
				delete(c.serviceTracker, lbID)
			}
		}
	}
	return reapplyRequests
}

func (c *UpdateTracker) GetIngressRequests() []reconcile.Request {
	lbs := c.currentState

	isCheck := make(map[string]bool)
	lbIDs := make([]string, 0)
	for key := range c.ingressTracker {
		lbIDs = append(lbIDs, key)
		isCheck[key] = false
	}
	logrus.Infof("Watching these ingress loadbalancers: %s.", strings.Join(lbIDs, ", "))

	reapplyRequests := make([]reconcile.Request, 0)
	for _, lbwt := range lbs {
		lb := lbwt
		if _, ok := c.ingressTracker[lb.UUID]; ok {
			isCheck[lb.UUID] = true
			for key := range c.ingressTracker[lb.UUID] {
				if c.ingressTracker[lb.UUID][key] != lb.UpdatedAt {
					logrus.Infof("Loadbalancer %s has been updated, sync now.", lb.UUID)
					namespace, name := revertKey(key)
					reapplyRequests = append(reapplyRequests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespace,
							Name:      name,
						},
					})
					delete(c.ingressTracker[lb.UUID], key)
				}
			}
		}
	}
	for lbID, value := range isCheck {
		if !value {
			logrus.Infof("Loadbalancer %s has been deleted, sync now.", lbID)
			for key := range c.ingressTracker[lbID] {
				namespace, name := revertKey(key)
				reapplyRequests = append(reapplyRequests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				})
				delete(c.ingressTracker, lbID)
			}
		}
	}
	return reapplyRequests
}
