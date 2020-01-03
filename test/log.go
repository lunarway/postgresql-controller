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
