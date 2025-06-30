package controller

import (
	"context"
	"reflect"

	"github.com/anngdinh/operator-helper/contexts"
	corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// func ensureClientObject(ctx context.Context, cl client.Client, obj client.Object) error {
// 	logger := contexts.NewContext(ctx).Log()

// 	// Check if the object exists
// 	objGet := obj.DeepCopyObject().(client.Object)
// 	err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, objGet)
// 	if err != nil && errors.IsNotFound(err) {
// 		logger.Infof("%s Creating object: %s", actionIcon, obj.GetName())
// 		return cl.Create(ctx, obj)
// 	} else if err != nil {
// 		logger.Errorf("Failed to get object: %v", err)
// 		return err
// 	}

// 	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
// 		objGet := obj.DeepCopyObject().(client.Object)
// 		if err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, objGet); err != nil {
// 			return err
// 		}

// 		logger.Infof("%s Patching object: %s/%s/%s", actionIcon, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
// 		return cl.Patch(ctx, obj, client.MergeFromWithOptions(objGet, client.MergeFromWithOptimisticLock{}))
// 	})
// }

func updateObjectAnnotation(ctx context.Context, cl client.Client, _obj client.Object, annotations map[string]string) error {
	logger := contexts.NewContext(ctx).Log()

	obj := _obj.DeepCopyObject().(client.Object)
	err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj)
	if err != nil {
		logger.Errorf("Failed to get object: %v", err)
		return err
	}

	isNeedUpdate := false
	for k, v := range annotations {
		if obj.GetAnnotations()[k] != v {
			isNeedUpdate = true
			break
		}
	}

	if !isNeedUpdate {
		logger.Debug("No need update annotations")
		return nil
	}

	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(annotations)
	} else {
		annos := obj.GetAnnotations()
		for k, v := range annotations {
			annos[k] = v
		}
		obj.SetAnnotations(annos)
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		objGet := obj.DeepCopyObject().(client.Object)
		if err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, objGet); err != nil {
			return err
		}

		logger.Infof("%s Updating object annotations: %s/%s/%s", actionIcon, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
		return cl.Patch(ctx, obj, client.MergeFromWithOptions(objGet, client.MergeFromWithOptimisticLock{}))
	})
}

// check if the node update condition, ignore LastTransitionTime, HeartbeatTime
func isNodeUpdateCondition(old, new *corev1.Node) bool {
	if old == nil && new == nil {
		return false
	}

	if old == nil || new == nil {
		return true
	}

	if old.Status.Conditions == nil && new.Status.Conditions == nil {
		return false
	}

	if old.Status.Conditions == nil || new.Status.Conditions == nil {
		return true
	}

	if len(old.Status.Conditions) != len(new.Status.Conditions) {
		return true
	}

	// compare each condition by type
	for i := range old.Status.Conditions {
		newCondition := func() *corev1.NodeCondition {
			for j := range new.Status.Conditions {
				if old.Status.Conditions[i].Type == new.Status.Conditions[j].Type {
					return &new.Status.Conditions[j]
				}
			}
			return nil
		}()

		if newCondition == nil {
			return true
		}

		if old.Status.Conditions[i].Status != newCondition.Status {
			return true
		}

		if old.Status.Conditions[i].Reason != newCondition.Reason {
			return true
		}

		if old.Status.Conditions[i].Message != newCondition.Message {
			return true
		}
	}

	return false
}

// check if the node update object meta, ignore ResourceVersion, Generation, ManagedFields
func isNodeUpdateObjectMeta(old, new *metav1.ObjectMeta) bool {
	if old == nil && new == nil {
		return false
	}

	if old == nil || new == nil {
		return true
	}

	oldClone := old.DeepCopy()
	newClone := new.DeepCopy()
	newClone.ResourceVersion = oldClone.ResourceVersion
	newClone.Generation = oldClone.Generation
	oldClone.ManagedFields = nil
	newClone.ManagedFields = nil

	return !reflect.DeepEqual(oldClone, newClone)
}
