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
	context string
	logr.Logger
}

func New(name, context string) Logger {
	return Logger{
		Logger:  log.Log.WithName(name),
		context: context,
	}
}

func (r *Logger) Info(message string, values ...interface{}) {
	r.Logger.Info(r.format(INFO, message, values))
}

func (r *Logger) Error(err error, message string, values ...interface{}) {
	r.Logger.Error(err, r.format(ERROR, message, values))
}

func (r *Logger) format(level, message string, values ...interface{}) string {
	return fmt.Sprintf(
		fmt.Sprintf(
			"[%s][%s] ",
			level,
			r.context)+message,
		values)
}
