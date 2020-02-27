package test

import (
	"io"
	"testing"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func SetLogger(t *testing.T) *log.DelegatingLogger {
	log.SetLogger(zap.LoggerTo(&logger{t: t}, true))
	return log.Log
}

func NewLogger(t *testing.T) logr.Logger {
	return zap.LoggerTo(&logger{t: t}, true)
}

var _ io.Writer = &logger{}

// logger is an io.Writer used for reporting logs to the test runner.
type logger struct {
	t *testing.T
}

func (l *logger) Write(p []byte) (int, error) {
	l.t.Logf("%s", p)
	return len(p), nil
}

// RawLogger implements a test logger that will log any info or log lines to a
// testing.T.Logf function.
type RawLogger struct {
	T      *testing.T
	name   string
	values []interface{}
}

var _ logr.Logger = &RawLogger{}

func (t *RawLogger) Info(msg string, keysAndValues ...interface{}) {
	t.T.Logf("INFO:  %s KEYPAIRS: %v", msg, keysAndValues)
}

func (t *RawLogger) Enabled() bool {
	return true
}

func (t *RawLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	t.T.Logf("ERROR: %s: %v KEYPAIRS: %v", msg, err, keysAndValues)
}

func (t *RawLogger) V(level int) logr.InfoLogger {
	return t
}

func (t *RawLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &RawLogger{
		T:      t.T,
		name:   t.name,
		values: append(t.values, keysAndValues...),
	}
}

func (t *RawLogger) WithName(name string) logr.Logger {
	return &RawLogger{
		T:      t.T,
		name:   name,
		values: t.values,
	}
}
