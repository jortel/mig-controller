package logging

import (
	"fmt"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	INFO  = "INFO"
	ERROR = "ERROR"
)

type Logger struct {
	component string
	wrapped   logr.Logger
}

func New(name, component string) Logger {
	return Logger{
		wrapped:   log.Log.WithName(name),
		component: component,
	}
}

func (r *Logger) V(level int) Logger {
	wrapped :=  r.wrapped.V(level).(logr.Logger)
	return Logger{
		component: r.component,
		wrapped:   wrapped,
	}
}

func (r *Logger) Info(message string, values ...interface{}) {
	r.wrapped.Info(r.format(INFO, message, values...))
}

func (r *Logger) Error(err error, message string, values ...interface{}) {
	r.wrapped.Error(err, r.format(ERROR, message, values...))
}

func (r *Logger) format(level, message string, values ...interface{}) string {
	header := fmt.Sprintf("[%s][%s] ", level, r.component)
	body := fmt.Sprintf(message, values...)
	return header+body
}
