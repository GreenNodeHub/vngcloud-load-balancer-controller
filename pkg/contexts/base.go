package contexts

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/sirupsen/logrus"
)

type logUtilsKey string

const keyLogID logUtilsKey = "id"
const keyName logUtilsKey = "name"

type IContext struct {
	context.Context
	logId string
	name  string

	mutex sync.Mutex
}

func NewContext(ctx context.Context) ContextWrapper {
	if ctx == nil {
		ctx = context.Background()
	}
	var logId, name string
	if value, ok := ctx.Value(keyLogID).(string); ok {
		logId = value
	} else {
		logId = randNumberWithThreeLetter()
		ctx = context.WithValue(ctx, keyLogID, logId)
	}
	if value, ok := ctx.Value(keyName).(string); ok {
		name = value
	}
	return &IContext{
		Context: ctx,
		logId:   logId,
		name:    name,
	}
}

func (s *IContext) Log() *logrus.Entry {
	fields := make(logrus.Fields)
	if s.logId != "" {
		fields[string(keyLogID)] = s.logId
	}
	if s.name != "" {
		fields[string(keyName)] = s.name
	}
	return logrus.WithFields(fields)
}

func (s *IContext) SetLogName(name string) ContextWrapper {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.name = name
	s.Context = context.WithValue(s.Context, keyName, name)
	return s
}

func (s *IContext) GetContext() context.Context {
	return s.Context
}

func (s *IContext) GetLogId() string {
	return s.logId
}

// ----------------------------------------------

func randNumberWithThreeLetter() string {
	number := randRange(100, 399)
	return fmt.Sprint(number)
}

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}
