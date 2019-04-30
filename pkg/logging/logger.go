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

// Logger that supports traditional logging but also provides for a consistent
// component and logging level prefix and Printf style logging.
type Logger struct {
	component string
	wrapped   logr.Logger
}

// Get a new logger for a component.  The component is most
// likely a controller name.
func New(name, component string) Logger {
	return Logger{
		wrapped:   log.Log.WithName(name),
		component: component,
	}
}

// Get a logger with verbosity.
func (r *Logger) V(level int) Logger {
	wrapped :=  r.wrapped.V(level).(logr.Logger)
	return Logger{
		component: r.component,
		wrapped:   wrapped,
	}
}

// Traditional info logging.
func (r *Logger) Info(message string, values ...interface{}) {
	r.wrapped.Info(r.format(INFO, message), values...)
}

// Traditional Error logging.
func (r *Logger) Error(err error, message string, values ...interface{}) {
	r.wrapped.Error(err, r.format(ERROR, message), values...)
}

// Printf Info logging.
func (r *Logger) Infof(message string, values ...interface{}) {
	r.wrapped.Info(r.format(INFO, message, values...))
}

// Printf Error logging.
func (r *Logger) Errorf(err error, message string, values ...interface{}) {
	r.wrapped.Error(err, r.format(ERROR, message, values...))
}

// Format the message; add the component and log level prefix.
func (r *Logger) format(level, message string, values ...interface{}) string {
	header := fmt.Sprintf("[%s][%s] ", level, r.component)
	body := fmt.Sprintf(message, values...)
	return header+body
}
