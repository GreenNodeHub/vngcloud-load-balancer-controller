package controller

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	successIcon = "✅"
	errorIcon   = "❌"
	actionIcon  = "🌐"
)

func genKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
func revertKey(key string) (string, string) {
	split := strings.Split(key, "/")
	return split[0], split[1]
}

func PointerOf[T any](t T) *T {
	return &t
}

func debugCompareMapString[T comparable](a, b map[string]T) {
	// get all keys
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}

	// log the difference
	for k := range keys {
		if k == "kubectl.kubernetes.io/last-applied-configuration" {
			continue
		}
		if a[k] != b[k] {
			logrus.Debugf("   + %s: (%v -> %v)", k, a[k], b[k])
		} else {
			logrus.Debugf("   - %s: %v", k, a[k])
		}
	}
}
