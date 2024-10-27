package contexts

import (
	"context"

	"github.com/sirupsen/logrus"
)

type ContextWrapper interface {
	context.Context

	Log() *logrus.Entry
	GetLogId() string
	SetLogName(name string) ContextWrapper

	GetContext() context.Context

	// AddMessage(message string) ContextWrapper
	// GetMessages() []string
	// ClearMessages() ContextWrapper
}
