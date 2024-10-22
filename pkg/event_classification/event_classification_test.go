package event_classification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockObject is a mock implementation of client.Object for testing purposes
type MockObject struct {
	client.Object
	name              string
	deletionTimestamp *v1.Time
}

func (m *MockObject) GetDeletionTimestamp() *v1.Time {
	return m.deletionTimestamp
}

func (m *MockObject) GetName() string {
	return m.name
}

func (m *MockObject) DeepCopyObject() runtime.Object {
	return m
}

// Test cases
func TestEventClassification_Classify(t *testing.T) {
	// Helper functions
	isValid := func(obj client.Object) bool {
		return obj != nil && obj.GetName() != ""
	}

	getResourceByKey := func(key string) (client.Object, bool) {
		if key == "valid-resource" {
			return &MockObject{name: "valid-resource"}, true
		}
		if key == "deleted-resource" {
			now := v1.Now()
			return &MockObject{name: "deleted-resource", deletionTimestamp: &now}, true
		}
		return nil, false
	}

	// Create an EventClassification instance
	ec := NewEventClassification(getResourceByKey, isValid)

	// Test CreateEvent
	t.Run("CreateEvent", func(t *testing.T) {
		event := ec.Classify("valid-resource")
		assert.NotNil(t, event)
		assert.Equal(t, CreateEvent, event.Type)
		assert.Equal(t, "valid-resource", event.Obj.GetName())
	})

	// Test DeleteEvent (when object is deleted in API, but exists in cache)
	t.Run("DeleteEventFromCache", func(t *testing.T) {
		ec.cache["deleted-resource"] = &MockObject{name: "deleted-resource"}
		event := ec.Classify("deleted-resource")
		assert.NotNil(t, event)
		assert.Equal(t, DeleteEvent, event.Type)
		assert.Equal(t, "deleted-resource", event.Obj.GetName())
	})

	// Test DeleteEvent (when resource has deletion timestamp)
	t.Run("DeleteEventWithDeletionTimestamp", func(t *testing.T) {
		event := ec.Classify("deleted-resource")
		assert.NotNil(t, event)
		assert.Equal(t, DeleteEvent, event.Type)
		assert.Equal(t, "deleted-resource", event.Obj.GetName())
	})

	// Test SyncEvent
	t.Run("SyncEvent", func(t *testing.T) {
		ec.cache["valid-resource"] = &MockObject{name: "valid-resource"}
		event := ec.Classify("valid-resource")
		assert.NotNil(t, event)
		assert.Equal(t, SyncEvent, event.Type)
		assert.Equal(t, "valid-resource", event.Obj.GetName())
		assert.Equal(t, "valid-resource", event.OldObj.GetName())
	})

	// Test invalid case (both objects invalid)
	t.Run("InvalidEvent", func(t *testing.T) {
		invalidGetResourceByKey := func(key string) (client.Object, bool) {
			return &MockObject{name: ""}, true
		}
		invalidEC := NewEventClassification(invalidGetResourceByKey, isValid)
		event := invalidEC.Classify("invalid-resource")
		assert.Nil(t, event)
	})
}
