package controller

import (
	"testing"
)

// Mock implementation of the getResourceByKey function for testing
func mockGetResourceByKey(mockData *map[string]interface{}) func(key string) (interface{}, bool) {
	return func(key string) (interface{}, bool) {
		obj, ok := (*mockData)[key]
		return obj, ok
	}
}

// Mock implementation of the isValid function for testing
func mockIsValid(validKeys map[string]bool) func(obj interface{}) bool {
	return func(obj interface{}) bool {
		str, ok := obj.(string)
		if !ok {
			return false
		}
		return validKeys[str]
	}
}

func TestEventClassification_Classify(t *testing.T) {
	// Initialize mock data
	mockData := map[string]interface{}{
		"resource1": "obj1",
		"resource2": "obj2",
	}
	validKeys := map[string]bool{
		"obj1":                 true,
		"obj1-updated-valid":   true,
		"obj1-updated-invalid": false,
		"obj2":                 true,
		"obj3":                 false,
	}

	ec := NewEventClassification(mockGetResourceByKey(&mockData), mockIsValid(validKeys))

	// Test CreateEvent: cache is empty, resource exists
	event := ec.Classify("resource1")
	if event == nil || event.Type != CreateEvent || event.Obj != "obj1" {
		t.Errorf("Expected CreateEvent for resource1, got %v", event)
	}

	// Test SyncEvent: object changed in cache to valid
	mockData["resource1"] = "obj1-updated-valid"
	event = ec.Classify("resource1")
	if event == nil || event.Type != SyncEvent || event.Obj != "obj1-updated-valid" {
		t.Errorf("Expected SyncEvent for resource1, got %v", event)
	}

	// Test DeleteEvent: object changed in cache to invalid
	mockData["resource1"] = "obj1-updated-invalid"
	event = ec.Classify("resource1")
	if event == nil || event.Type != DeleteEvent || event.Obj != "obj1-updated-valid" {
		t.Errorf("Expected DeleteEvent for resource1, got %v", event)
	}

	// Test UpdateEvent: object changed in cache to valid
	mockData["resource1"] = "obj1-updated-valid"
	event = ec.Classify("resource1")
	if event == nil || event.Type != CreateEvent || event.Obj != "obj1-updated-valid" {
		t.Errorf("Expected CreateEvent for resource1, got %v", event)
	}

	// Test DeleteEvent: resource was removed from external data
	delete(mockData, "resource1")
	event = ec.Classify("resource1")
	if event == nil || event.Type != DeleteEvent || event.Obj != "obj1-updated-valid" {
		t.Errorf("Expected DeleteEvent for resource1, got %v", event)
	}

	// Test nil: new object with invalid state
	mockData["resource3"] = "obj3"
	event = ec.Classify("resource3")
	if event != nil {
		t.Errorf("Expected nil for resource3, got %v", event)
	}

	// Test nil: no event triggered for an unknown resource
	event = ec.Classify("unknown")
	if event != nil {
		t.Errorf("Expected nil for unknown resource, got %v", event)
	}
}

func TestEventClassification_Classify2(t *testing.T) {
	tests := []struct {
		name             string
		key              string
		cache            map[string]interface{}
		getResourceByKey func(key string) (interface{}, bool)
		isValid          func(obj interface{}) bool
		expectedType     EventType
		expectedObj      interface{}
		expectedOldObj   interface{}
	}{
		{
			name:  "Create event when object exists in resource but not in cache",
			key:   "testKey",
			cache: map[string]interface{}{},
			getResourceByKey: func(key string) (interface{}, bool) {
				if key == "testKey" {
					return "resourceObject", true
				}
				return nil, false
			},
			isValid:        func(obj interface{}) bool { return true },
			expectedType:   CreateEvent,
			expectedObj:    "resourceObject",
			expectedOldObj: nil,
		},
		{
			name: "Delete event when object exists in cache but not in resource",
			key:  "testKey",
			cache: map[string]interface{}{
				"testKey": "cachedObject",
			},
			getResourceByKey: func(key string) (interface{}, bool) {
				return nil, false
			},
			isValid:        func(obj interface{}) bool { return true },
			expectedType:   DeleteEvent,
			expectedObj:    "cachedObject",
			expectedOldObj: nil,
		},
		{
			name: "Sync event when object exists in both cache and resource",
			key:  "testKey",
			cache: map[string]interface{}{
				"testKey": "cachedObject",
			},
			getResourceByKey: func(key string) (interface{}, bool) {
				return "cachedObject", true
			},
			isValid:        func(obj interface{}) bool { return true },
			expectedType:   SyncEvent,
			expectedObj:    "cachedObject",
			expectedOldObj: nil,
		},
		{
			name: "Create event when cached object is invalid and resource object is valid",
			key:  "testKey",
			cache: map[string]interface{}{
				"testKey": "invalidObject",
			},
			getResourceByKey: func(key string) (interface{}, bool) {
				return "resourceObject", true
			},
			isValid: func(obj interface{}) bool {
				return obj != "invalidObject"
			},
			expectedType:   CreateEvent,
			expectedObj:    "resourceObject",
			expectedOldObj: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec := &EventClassification{
				cache:            tt.cache,
				getResourceByKey: tt.getResourceByKey,
				isValid:          tt.isValid,
			}

			event := ec.Classify(tt.key)

			if event == nil || event.Type != tt.expectedType {
				t.Errorf("expected event type %v, got %v", tt.expectedType, event.Type)
			}

			if event.Obj != tt.expectedObj {
				t.Errorf("expected event object %v, got %v", tt.expectedObj, event.Obj)
			}
		})
	}
}
