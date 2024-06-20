package utils

import (
	"github.com/go-logr/logr"
)

type ProviderStatus interface {
	ToStatusMap() (map[string]string, error)
}

type AssertLogger struct {
	logr.Logger
}

func (t AssertLogger) Errorf(msg string, args ...interface{}) {
	t.Error(nil, msg, args...)
}

type ProviderContextKey struct{}
