package shared

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddFinalizer(obj client.Object, f string) bool {
	for _, x := range obj.GetFinalizers() {
		if x == f {
			return false
		}
	}
	obj.SetFinalizers(append(obj.GetFinalizers(), f))
	return true
}

func RemoveFinalizer(obj client.Object, f string) bool {
	out := make([]string, 0, len(obj.GetFinalizers()))
	found := false
	for _, x := range obj.GetFinalizers() {
		if x == f {
			found = true
			continue
		}
		out = append(out, x)
	}
	if !found {
		return false
	}
	obj.SetFinalizers(out)
	return true
}

func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object, f string) error {
	if !AddFinalizer(obj, f) {
		return nil
	}
	return c.Update(ctx, obj)
}
