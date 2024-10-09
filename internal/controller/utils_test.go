package controller

import (
	"testing"
)

func TestGenKey(t *testing.T) {
	namespace := "default"
	name := "my-app"
	expected := "default/my-app"
	key := genKey(namespace, name)

	if key != expected {
		t.Errorf("genKey(%s, %s) = %s; want %s", namespace, name, key, expected)
	}
}

func TestRevertKey(t *testing.T) {
	key := "default/my-app"
	expectedNamespace := "default"
	expectedName := "my-app"
	namespace, name := revertKey(key)

	if namespace != expectedNamespace || name != expectedName {
		t.Errorf("revertKey(%s) = (%s, %s); want (%s, %s)", key, namespace, name, expectedNamespace, expectedName)
	}
}

func TestPointerOf(t *testing.T) {
	val := 42
	ptr := PointerOf(val)

	if *ptr != val {
		t.Errorf("PointerOf(%d) = %d; want %d", val, *ptr, val)
	}

	str := "hello"
	ptrStr := PointerOf(str)

	if *ptrStr != str {
		t.Errorf("PointerOf(%s) = %s; want %s", str, *ptrStr, str)
	}
}
