package event_classification

import (
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventType string

const (
	CreateEvent EventType = "CREATE"
	DeleteEvent EventType = "DELETE"
	SyncEvent   EventType = "SYNC"
)

// Event holds the context of an event
type Event struct {
	Type   EventType
	Obj    client.Object
	OldObj client.Object
}

type EventClassification struct {
	cache            map[string]client.Object
	getResourceByKey func(key string) (client.Object, bool)
	isValid          func(obj client.Object) bool
}

func NewEventClassification(getResourceByKey func(key string) (client.Object, bool), isValid func(obj client.Object) bool) *EventClassification {
	return &EventClassification{
		cache:            make(map[string]client.Object),
		getResourceByKey: getResourceByKey,
		isValid:          isValid,
	}
}

func (ec *EventClassification) Classify(key string) *Event {
	objGet, okGet := ec.getResourceByKey(key)
	objCache, okCache := ec.cache[key]

	objGetValid, objCacheValid := ec.isValid(objGet), ec.isValid(objCache)

	// logrus.Infof("objGet: %v, objCache: %v", objGet, objCache)
	logrus.Debugf("okGet:   %v, objGetValid:   %v", okGet, objGetValid)
	logrus.Debugf("okCache: %v, objCacheValid: %v", okCache, objCacheValid)

	// if objGet is deleted, but objCache is exist, then delete
	if okCache && !okGet {
		delete(ec.cache, key)
		return &Event{
			Type: DeleteEvent,
			Obj:  objCache,
		}
	}

	// if objGet, objCache is exist, but objGet have deletionTimestamp, then delete
	if okGet && isHaveDeleteTimestamp(objGet) {
		delete(ec.cache, key)
		return &Event{
			Type: DeleteEvent,
			Obj:  objGet,
		}
	}

	if !okCache && okGet {
		if !objGetValid {
			return nil
		}
		ec.cache[key] = objGet
		return &Event{
			Type: CreateEvent,
			Obj:  objGet,
		}
	}

	if !okCache && !okGet {
		return nil
	}

	if okCache && okGet {
		if !objGetValid && !objCacheValid {
			return nil
		}

		if !objGetValid {
			delete(ec.cache, key)
			return &Event{
				Type: DeleteEvent,
				Obj:  objCache,
			}
		}

		if !objCacheValid {
			ec.cache[key] = objGet
			return &Event{
				Type: CreateEvent,
				Obj:  objGet,
			}
		}
	}

	// okCache && okGet && objGetValid && objCacheValid
	ec.cache[key] = objGet
	return &Event{
		Type:   SyncEvent,
		Obj:    objGet,
		OldObj: objCache,
	}
}

func isHaveDeleteTimestamp(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}
