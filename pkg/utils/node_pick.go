package utils

import (
	corev1 "k8s.io/api/core/v1"
)

// FirstNodeByName returns the node with the lexicographically smallest name, or nil if the
// list is empty.
//
// Callers use one node to stand for the whole cluster - its network, subnet and zone become
// the defaults new load balancers are created in - and then latch that for the lifetime of the
// process. Taking nodes.Items[0] made that choice depend on the order a List happened to
// return, which for a cached client is the informer's own order and is not defined: a
// different node early in a process's life, a different one again after a restart, and with it
// a different subnet for every load balancer created afterwards.
//
// Sorting by name does not make the choice any more *correct* - any node in the cluster is as
// good a witness as another - but it makes it the same every time, which is what a default
// has to be.
func FirstNodeByName(nodes *corev1.NodeList) *corev1.Node {
	if nodes == nil || len(nodes.Items) == 0 {
		return nil
	}

	first := &nodes.Items[0]
	for i := range nodes.Items {
		if nodes.Items[i].Name < first.Name {
			first = &nodes.Items[i]
		}
	}
	return first
}
