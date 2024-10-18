package controller

import (
	"fmt"
	"strings"
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

// Generic function to remove the first occurrence of a matching value from a slice
func removeFisrt[T comparable](slice []T, value T) []T {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
