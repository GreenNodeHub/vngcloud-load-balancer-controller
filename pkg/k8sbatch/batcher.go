package k8sbatch

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Batcher coalesces queued mutations against Kubernetes objects so a single
// reconcile performs at most one GET and one PATCH per object per
// sub-resource. Not safe for concurrent use; reconciles are single-goroutine
// per object key in controller-runtime.
type Batcher struct {
	client  client.Client
	pending map[objKey]*entry
}

// objKey identifies an object by GVK + namespace/name.
type objKey struct {
	gvk schema.GroupVersionKind
	nn  types.NamespacedName
}

// entry holds the queued mutators for one logical object plus a template
// pointer used to issue fresh GETs.
type entry struct {
	template       client.Object
	specMutators   []func(client.Object) bool
	statusMutators []func(client.Object) bool
}

func New(c client.Client) *Batcher {
	return &Batcher{
		client:  c,
		pending: make(map[objKey]*entry),
	}
}

// Pending returns the number of distinct objects currently queued.
func (b *Batcher) Pending() int { return len(b.pending) }
