package controller

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"
)

type logUtilsKey string

const LogUtilsName logUtilsKey = "name"

type logHelper struct {
	keys map[logUtilsKey]bool
	sync.Mutex
}

func (l *logHelper) RegisterKey(key logUtilsKey) {
	l.Lock()
	defer l.Unlock()
	l.keys[key] = true
}

func (l *logHelper) LogWithContext(ctx context.Context) *logrus.Entry {
	fields := make(logrus.Fields)
	for key := range l.keys {
		if value, ok := ctx.Value(key).(string); ok {
			fields[string(key)] = value
		}
	}
	logrus.Println("LogWithContext fields: ", fields)
	return logrus.WithFields(fields)
}

// Singleton instance of logHelper
var instance *logHelper
var once sync.Once

// GetLogUtils returns the singleton instance of logHelper
func GetLogUtils() *logHelper {
	once.Do(func() {
		// Initialize the singleton logHelper
		instance = &logHelper{
			keys: make(map[logUtilsKey]bool),
		}
	})
	instance.RegisterKey(LogUtilsName)

	return instance
}
