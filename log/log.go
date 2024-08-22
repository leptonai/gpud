// Package log provides the logging functionality for gpud.
package log

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *LeptonLogger
)

func init() {
	Logger = CreateLogger(DefaultLoggerConfig())
}

func DefaultLoggerConfig() *zap.Config {
	c := zap.NewProductionConfig()
	c.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	return &c
}

func CreateLogger(config *zap.Config) *LeptonLogger {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	l, err := config.Build()
	if err != nil {
		panic(err)
	}

	return &LeptonLogger{
		l.Sugar(),
	}
}

type LeptonLogger struct {
	*zap.SugaredLogger
}

// Override the default logger's Errorw func to down level context canceled error
func (l *LeptonLogger) Errorw(msg string, keysAndValues ...interface{}) {
	for i := 0; i < len(keysAndValues); i += 2 {
		if keysAndValues[i] != "error" {
			continue
		}
		if err, ok := keysAndValues[i+1].(error); ok {
			if strings.Contains(err.Error(), context.Canceled.Error()) {
				l.SugaredLogger.Warnw(msg, keysAndValues...)
				return
			}
		}
	}

	l.SugaredLogger.Errorw(msg, keysAndValues...)
}
