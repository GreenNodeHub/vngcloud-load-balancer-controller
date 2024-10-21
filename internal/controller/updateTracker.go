package controller

import (
	"strings"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UpdateTracker struct {
	tracker map[string]map[string]string
}

func NewUpdateTracker() *UpdateTracker {
	return &UpdateTracker{
		tracker: make(map[string]map[string]string),
	}
}

func (c *UpdateTracker) AddUpdateTracker(lbID, namespace, name, updateAt string) {
	if _, ok := c.tracker[lbID]; !ok {
		c.tracker[lbID] = make(map[string]string)
		c.tracker[lbID][genKey(namespace, name)] = updateAt
	} else {
		if _, ok := c.tracker[lbID][genKey(namespace, name)]; !ok {
			c.tracker[lbID][genKey(namespace, name)] = updateAt
		} else {
			c.tracker[lbID][genKey(namespace, name)] = updateAt
		}
	}
}

func (c *UpdateTracker) RemoveUpdateTracker(lbID, namespace, name string) {
	if _, ok := c.tracker[lbID]; ok {
		delete(c.tracker[lbID], genKey(namespace, name))
		if len(c.tracker[lbID]) == 0 {
			delete(c.tracker, lbID)
		}
	}
}

func (c *UpdateTracker) GetReconcileRequests(lbs *entityv2.ListLoadBalancers) []reconcile.Request {
	isCheck := make(map[string]bool)
	lbIDs := make([]string, 0)
	for key := range c.tracker {
		lbIDs = append(lbIDs, key)
		isCheck[key] = false
	}
	logrus.Debugf("Watching these loadbalancers: %s.", strings.Join(lbIDs, ", "))

	reapplyRequests := make([]reconcile.Request, 0)
	for _, lb := range lbs.Items {
		if _, ok := c.tracker[lb.UUID]; ok {
			isCheck[lb.UUID] = true
			for key := range c.tracker[lb.UUID] {
				if c.tracker[lb.UUID][key] != lb.UpdatedAt {
					logrus.Infof("Loadbalancer %s has been updated, sync now.", lb.UUID)
					namespace, name := revertKey(key)
					reapplyRequests = append(reapplyRequests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespace,
							Name:      name,
						},
					})
					delete(c.tracker[lb.UUID], key)
				}
			}
		}
	}
	for lbID, value := range isCheck {
		if !value {
			logrus.Infof("Loadbalancer %s has been deleted, sync now.", lbID)
			for key := range c.tracker[lbID] {
				namespace, name := revertKey(key)
				reapplyRequests = append(reapplyRequests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				})
				delete(c.tracker, lbID)
			}
		}
	}
	if len(reapplyRequests) == 0 {
		logrus.Debug("Nothing change.")
	}
	return reapplyRequests
}
