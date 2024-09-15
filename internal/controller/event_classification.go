package controller

import (
// "github.com/sirupsen/logrus"
)

type EventType string

const (
	CreateEvent EventType = "CREATE"
	// UpdateEvent EventType = "UPDATE"
	DeleteEvent EventType = "DELETE"
	SyncEvent   EventType = "SYNC"
)

// Event holds the context of an event
type Event struct {
	Type EventType
	Obj  interface{}
}

type EventClassification struct {
	cache            map[string]interface{}
	getResourceByKey func(key string) (interface{}, bool)
	isValid          func(obj interface{}) bool
}

func NewEventClassification(getResourceByKey func(key string) (interface{}, bool), isValid func(obj interface{}) bool) *EventClassification {
	return &EventClassification{
		cache:            make(map[string]interface{}),
		getResourceByKey: getResourceByKey,
		isValid:          isValid,
	}
}

func (ec *EventClassification) Classify(key string) *Event {
	objGet, okGet := ec.getResourceByKey(key)
	objCache, okCache := ec.cache[key]

	objGetValid, objCacheValid := ec.isValid(objGet), ec.isValid(objCache)

	// logrus.Infof("okGet: %v, objGetValid: %v", okGet, objGetValid)
	// logrus.Infof("okCache: %v, objCacheValid: %v", okCache, objCacheValid)

	if okCache && !okGet {
		delete(ec.cache, key)
		return &Event{
			Type: DeleteEvent,
			Obj:  objCache,
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

	ec.cache[key] = objGet
	return &Event{
		Type: SyncEvent,
		Obj:  objGet,
	}
}
