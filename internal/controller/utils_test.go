package controller

// import (
// 	"reflect"
// 	"testing"
// )

// func TestGenKey(t *testing.T) {
// 	namespace := "default"
// 	name := "my-app"
// 	expected := "default/my-app"
// 	key := genKey(namespace, name)

// 	if key != expected {
// 		t.Errorf("genKey(%s, %s) = %s; want %s", namespace, name, key, expected)
// 	}
// }

// func TestRevertKey(t *testing.T) {
// 	key := "default/my-app"
// 	expectedNamespace := "default"
// 	expectedName := "my-app"
// 	namespace, name := revertKey(key)

// 	if namespace != expectedNamespace || name != expectedName {
// 		t.Errorf("revertKey(%s) = (%s, %s); want (%s, %s)", key, namespace, name, expectedNamespace, expectedName)
// 	}
// }

// func TestPointerOf(t *testing.T) {
// 	val := 42
// 	ptr := PointerOf(val)

// 	if *ptr != val {
// 		t.Errorf("PointerOf(%d) = %d; want %d", val, *ptr, val)
// 	}

// 	str := "hello"
// 	ptrStr := PointerOf(str)

// 	if *ptrStr != str {
// 		t.Errorf("PointerOf(%s) = %s; want %s", str, *ptrStr, str)
// 	}
// }

// // Test for int slice
// func TestRemoveInt(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    []int
// 		value    int
// 		expected []int
// 	}{
// 		{
// 			name:     "Remove first occurrence of 3",
// 			input:    []int{1, 2, 3, 4, 3, 5},
// 			value:    3,
// 			expected: []int{1, 2, 4, 3, 5},
// 		},
// 		{
// 			name:     "Remove 1 from start",
// 			input:    []int{1, 2, 3},
// 			value:    1,
// 			expected: []int{2, 3},
// 		},
// 		{
// 			name:     "Remove non-existent value",
// 			input:    []int{1, 2, 3},
// 			value:    4,
// 			expected: []int{1, 2, 3},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := removeFisrt(tt.input, tt.value)
// 			if !reflect.DeepEqual(result, tt.expected) {
// 				t.Errorf("Expected %v, got %v", tt.expected, result)
// 			}
// 		})
// 	}
// }

// // Test for string slice
// func TestRemoveString(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    []string
// 		value    string
// 		expected []string
// 	}{
// 		{
// 			name:     "Remove first occurrence of 'b'",
// 			input:    []string{"a", "b", "c", "b", "d"},
// 			value:    "b",
// 			expected: []string{"a", "c", "b", "d"},
// 		},
// 		{
// 			name:     "Remove 'a' from start",
// 			input:    []string{"a", "b", "c"},
// 			value:    "a",
// 			expected: []string{"b", "c"},
// 		},
// 		{
// 			name:     "Remove non-existent value",
// 			input:    []string{"a", "b", "c"},
// 			value:    "z",
// 			expected: []string{"a", "b", "c"},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := removeFisrt(tt.input, tt.value)
// 			if !reflect.DeepEqual(result, tt.expected) {
// 				t.Errorf("Expected %v, got %v", tt.expected, result)
// 			}
// 		})
// 	}
// }
