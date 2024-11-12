package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UpdateTrackerInterface interface {
	Start(context.Context, string)

	AddService(lbID, updateAt string, service *corev1.Service)
	RemoveService(lbID string, service *corev1.Service)
	// GetServiceRequests returns a list of reconcile requests for services that need to be re-applied
	// - lb is updated
	// - lb is deleted
	// - lb is updated tags
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

	// contains all load balancers with VKS_TAG_KEY tag and cluster ID
	currentState []*LoadBalancerWithTags
	provider     provider.Provider
	clusterID    string

	once sync.Once
	mu   sync.Mutex
}

func NewUpdateTracker(provider provider.Provider) *UpdateTracker {
	return &UpdateTracker{
		serviceTracker: make(map[string]map[string]string),
		ingressTracker: make(map[string]map[string]string),
		currentState:   make([]*LoadBalancerWithTags, 0),
		provider:       provider,
	}
}

func (c *UpdateTracker) Start(ctx context.Context, clusterID string) {
	c.clusterID = clusterID
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
	lbs, err := c.provider.ListLoadBalancers(ctx, []string{consts.VKS_TAG_KEY})
	if err != nil {
		return err
	}

	state := make([]*LoadBalancerWithTags, 0)
	for _, lb := range lbs.Items {
		tags, err := c.provider.ListTags(ctx, lb.UUID)
		if err != nil {
			return err
		}

		// Filter out load balancers that have the VKS_TAG_KEY tag containing the cluster ID
		isBelongToCluster := false
		for _, tag := range tags.Items {
			if tag.Key == consts.VKS_TAG_KEY && strings.Contains(tag.Value, c.clusterID) {
				isBelongToCluster = true
				break
			}
		}
		if !isBelongToCluster {
			continue
		}
		state = append(state, &LoadBalancerWithTags{
			LoadBalancer: lb,
			Tag:          tags.Items,
		})
	}
	c.currentState = state
	return nil
}

func (c *UpdateTracker) AddService(lbID, updateAt string, service *corev1.Service) {
	c.addObject(lbID, updateAt, service, c.serviceTracker)
}

func (c *UpdateTracker) AddIngress(lbID, updateAt string, ingress *networkingv1.Ingress) {
	c.addObject(lbID, updateAt, ingress, c.ingressTracker)
}

func (c *UpdateTracker) addObject(lbID, updateAt string, obj client.Object, tracker map[string]map[string]string) {
	objKey := genKey(obj.GetNamespace(), obj.GetName())

	// delete this key if it exists in tracker
	for _, value := range tracker {
		delete(value, objKey)
	}

	if _, ok := tracker[lbID]; !ok {
		tracker[lbID] = make(map[string]string)
		tracker[lbID][objKey] = updateAt
	} else {
		if _, ok := tracker[lbID][objKey]; !ok {
			tracker[lbID][objKey] = updateAt
		} else {
			tracker[lbID][objKey] = updateAt
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
	lbs := make([]*LoadBalancerWithTags, 0)
	for _, lb := range c.currentState {
		if lb.LoadBalancer.Type == "Layer 4" {
			lbs = append(lbs, lb)
		}
	}

	requests := c.getRequests(lbs, c.serviceTracker)
	if len(requests) == 0 {
		logrus.Infof("Watching these NLBs: %+v", c.serviceTracker)
	}
	return requests
}

func (c *UpdateTracker) GetIngressRequests() []reconcile.Request {
	lbs := make([]*LoadBalancerWithTags, 0)
	for _, lb := range c.currentState {
		if lb.LoadBalancer.Type == "Layer 7" {
			lbs = append(lbs, lb)
		}
	}

	requests := c.getRequests(lbs, c.ingressTracker)
	if len(requests) == 0 {
		logrus.Infof("Watching these ALBs: %+v", c.ingressTracker)
	}
	return requests
}

// Expecting that all lb in lbs are equal to all lb in tracker, otherwise
// if missing lb in lbs, it has been deleted or updated tags
// if redundant lb in lbs, this lb has redundant tags
func (c *UpdateTracker) getRequests(lbs []*LoadBalancerWithTags, tracker map[string]map[string]string) []reconcile.Request {
	getByID := func(lbID string) *LoadBalancerWithTags {
		for _, lb := range lbs {
			if lb.LoadBalancer.UUID == lbID {
				return lb
			}
		}
		return nil
	}

	// loop through all load balancers in tracker
	// if lb is not in lbs, it has been deleted or updated tags
	// if lb is in lbs, check if it has been updated
	reapplyRequests := make([]reconcile.Request, 0)
	for lbID := range tracker {
		if len(tracker[lbID]) == 0 {
			delete(tracker, lbID)
			continue
		}
		lb := getByID(lbID)

		// lb has been deleted or updated tags
		if lb == nil {
			logrus.Infof("Loadbalancer %s has been deleted or updated tags, sync now.", lbID)
			for key := range tracker[lbID] {
				namespace, name := revertKey(key)
				reapplyRequests = append(reapplyRequests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				})
			}
			delete(tracker, lbID)
			continue
		}

		// check if lb has been updated
		for key := range tracker[lbID] {
			if tracker[lbID][key] != lb.LoadBalancer.UpdatedAt {
				logrus.Infof("Loadbalancer %s has been updated, sync now.", lbID)
				namespace, name := revertKey(key)
				reapplyRequests = append(reapplyRequests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				})
				delete(tracker[lbID], key)
			}
		}
	}

	// check if there is any redundant lb in lbs
	for _, lb := range lbs {
		if _, ok := tracker[lb.LoadBalancer.UUID]; !ok {

			currentTags := make(map[string]string)
			newTags := make(map[string]string)
			for _, tag := range lb.Tag {
				currentTags[tag.Key] = tag.Value
				if tag.Key != consts.VKS_TAG_KEY {
					newTags[tag.Key] = tag.Value
					continue
				}
				// remove cluster ID in tag value
				clusterIDs := strings.Split(tag.Value, consts.VKS_TAGS_SEPARATOR)
				newTagValue := make([]string, 0)
				for _, id := range clusterIDs {
					if id != c.clusterID && id != "" {
						newTagValue = append(newTagValue, id)
					}
				}
				v := strings.Join(newTagValue, consts.VKS_TAGS_SEPARATOR)
				if len(v) < 3 || len(v) > 255 {
					continue
				}
				newTags[tag.Key] = strings.Join(newTagValue, consts.VKS_TAGS_SEPARATOR)
			}

			logrus.Infof("Loadbalancer %s has redundant tags, edit tags %+v -> %+v.", lb.LoadBalancer.UUID, currentTags, newTags)
			// update tags
			err := c.provider.CreateTags(context.Background(), lb.LoadBalancer.UUID, newTags)
			if err != nil {
				logrus.Errorf("Failed to update tags: %v", err)
			}
			logrus.Infof("Successfully updated redundant tags for loadbalancer %s.", lb.LoadBalancer.UUID)
		}
	}

	return reapplyRequests
}
