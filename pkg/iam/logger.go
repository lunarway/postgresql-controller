package iam

import (
	"testing"

	"github.com/go-logr/logr"
)

type TestLogger struct {
	T      *testing.T
	name   string
	values []interface{}
}

var _ logr.Logger = &TestLogger{}

func NewLogger(T *testing.T) *TestLogger {
	return &TestLogger{
		T:      T,
		name:   "test",
		values: nil,
	}
}

func (t *TestLogger) Info(msg string, keysAndValues ...interface{}) {
	t.T.Logf("INFO:  %s KEYPAIRS: %v", msg, keysAndValues)
}

func (t *TestLogger) Enabled() bool {
	return true
}

func (t *TestLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	t.T.Logf("ERROR: %s: %v KEYPAIRS: %v", msg, err, keysAndValues)
}

func (t *TestLogger) V(level int) logr.InfoLogger {
	return t
}

func (t *TestLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &TestLogger{
		T:      t.T,
		name:   t.name,
		values: append(t.values, keysAndValues...),
	}
}

func (t *TestLogger) WithName(name string) logr.Logger {
	return &TestLogger{
		T:      t.T,
		name:   name,
		values: t.values,
	}
}
