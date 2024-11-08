package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger to provide logging functionality
type Logger struct {
	*zap.SugaredLogger
}

// Global logger for convenience
var L *Logger

// NewLogger creates and returns a new Logger instance
func NewLogger() (*Logger, error) {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{
		SugaredLogger: zapLogger.Sugar(),
	}, nil
}

// Initialize default logger and set it as global while also using Dependency Injection
// Given logger is a heavily used object and is used in many places so it's a good idea to 
// have it as a global variable as well for usecases like scripts but for everywhere else
// we should try to use the Dependency Injection approach only.
func init() {
    L, _ = NewLogger()
}

// Helper methods to make logging more convenient
func (l *Logger) Debugf(template string, args ...interface{}) {
	l.SugaredLogger.Debugf(template, args...)
}

func (l *Logger) Infof(template string, args ...interface{}) {
	l.SugaredLogger.Infof(template, args...)
}

func (l *Logger) Warnf(template string, args ...interface{}) {
	l.SugaredLogger.Warnf(template, args...)
}

func (l *Logger) Errorf(template string, args ...interface{}) {
	l.SugaredLogger.Errorf(template, args...)
}

func (l *Logger) Fatalf(template string, args ...interface{}) {
	l.SugaredLogger.Fatalf(template, args...)
}
