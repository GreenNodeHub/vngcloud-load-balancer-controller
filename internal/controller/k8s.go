package controller

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ensureClientObject(ctx context.Context, cl client.Client, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()

	// Check if the object exists
	objGet := obj.DeepCopyObject().(client.Object)
	err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, objGet)
	if err != nil && errors.IsNotFound(err) {
		logger.Infof("%s Creating object: %s", actionIcon, obj.GetName())
		return cl.Create(ctx, obj)
	} else if err != nil {
		logger.Errorf("Failed to get object: %v", err)
		return err
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		objGet := obj.DeepCopyObject().(client.Object)
		if err := cl.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, objGet); err != nil {
			return err
		}

		logger.Infof("%s Patching object: %s/%s/%s", actionIcon, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
		return cl.Patch(ctx, obj, client.MergeFromWithOptions(objGet, client.MergeFromWithOptimisticLock{}))
	})
}

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
