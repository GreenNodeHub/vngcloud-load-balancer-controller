package util

import (
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type ReconcileCounters struct {
	serviceReconciles map[types.NamespacedName]int
	ingressReconciles map[types.NamespacedName]int
	lbcReconciles     map[types.NamespacedName]int
	glbcReconciles    map[types.NamespacedName]int
	nsgReconciles     map[types.NamespacedName]int
	mutex             sync.Mutex
}

type ResourceReconcileCount struct {
	Resource types.NamespacedName
	Count    int
}

func NewReconcileCounters() *ReconcileCounters {
	return &ReconcileCounters{
		serviceReconciles: make(map[types.NamespacedName]int),
		ingressReconciles: make(map[types.NamespacedName]int),
		lbcReconciles:     make(map[types.NamespacedName]int),
		glbcReconciles:    make(map[types.NamespacedName]int),
		nsgReconciles:     make(map[types.NamespacedName]int),
		mutex:             sync.Mutex{},
	}
}

func (c *ReconcileCounters) IncrementService(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.serviceReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementIngress(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.ingressReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementLbc(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.lbcReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementGlbc(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.glbcReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementNsg(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nsgReconciles[namespaceName]++
}

func (c *ReconcileCounters) GetTopReconciles(n int) map[string][]ResourceReconcileCount {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	topReconciles := make(map[string][]ResourceReconcileCount)
	getTopN := func(m map[types.NamespacedName]int) []ResourceReconcileCount {
		reconciles := make([]ResourceReconcileCount, 0, len(m))
		for k, v := range m {
			reconciles = append(reconciles, ResourceReconcileCount{Resource: k, Count: v})
		}

		sort.Slice(reconciles, func(i, j int) bool {
			return reconciles[i].Count > reconciles[j].Count
		})
		if len(reconciles) > n {
			reconciles = reconciles[:n]
		}
		return reconciles
	}

	topReconciles["service"] = getTopN(c.serviceReconciles)
	topReconciles["ingress"] = getTopN(c.ingressReconciles)

	return topReconciles
}

func (c *ReconcileCounters) ResetCounter() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.serviceReconciles = make(map[types.NamespacedName]int)
	c.ingressReconciles = make(map[types.NamespacedName]int)
	c.lbcReconciles = make(map[types.NamespacedName]int)
	c.glbcReconciles = make(map[types.NamespacedName]int)
	c.nsgReconciles = make(map[types.NamespacedName]int)
}
