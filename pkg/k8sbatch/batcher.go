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
// pointer used to issue fresh GETs at flush time.
type entry struct {
	// template is the *T pointer captured from the first MutateSpec/
	// MutateStatus call for this objKey. DeepCopyObject on it must return
	// the same concrete type because the wrapped mutators below perform
	// an unchecked o.(T) type assertion on the passed client.Object.
	template client.Object

	// specMutators and statusMutators are closures that wrap a typed
	// func(*T) bool and perform an unchecked .(T) assertion on the
	// passed client.Object. Two MutateSpec/MutateStatus calls with the
	// same objKey but different concrete types would panic — but objKey
	// includes GVK, so this can only happen via API misuse.
	specMutators   []func(client.Object) bool
	statusMutators []func(client.Object) bool
}

// New returns a Batcher backed by c. The returned Batcher is not safe for
// concurrent use; reconciles are single-goroutine per object key in
// controller-runtime.
func New(c client.Client) *Batcher {
	return &Batcher{
		client:  c,
		pending: make(map[objKey]*entry),
	}
}

// Pending returns the number of distinct objects currently queued.
func (b *Batcher) Pending() int { return len(b.pending) }
