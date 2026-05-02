package ownerevents

import (
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// RecordEventOnOwner records an event on the resource that owns obj (if any).
// The owner is identified by the standard owner labels set on obj:
//   - domain.LabelOwnerResourceKind
//   - domain.LabelOwnerResourceName
//   - domain.LabelOwnerResourceUid
//
// The owner Kind is resolved generically via restMapper, so no switch on specific
// types is required. Silently does nothing if labels are absent or the Kind is unknown.
func RecordEventOnOwner(
	restMapper apimeta.RESTMapper,
	eventRecorder record.EventRecorder,
	obj client.Object,
	eventType, reason, message string,
) {
	labels := obj.GetLabels()
	ownerKind := labels[domain.LabelOwnerResourceKind]
	ownerName := labels[domain.LabelOwnerResourceName]
	ownerUID := labels[domain.LabelOwnerResourceUid]
	if ownerKind == "" || ownerName == "" {
		return
	}

	mapping, err := restMapper.RESTMapping(schema.GroupKind{Kind: ownerKind})
	if err != nil {
		return
	}

	owner := &unstructured.Unstructured{}
	owner.SetAPIVersion(mapping.GroupVersionKind.GroupVersion().String())
	owner.SetKind(ownerKind)
	owner.SetNamespace(obj.GetNamespace())
	owner.SetName(ownerName)
	owner.SetUID(types.UID(ownerUID))

	eventRecorder.Event(owner, eventType, reason, message)
}
