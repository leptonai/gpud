// Package log provides the logging functionality for gpud.
package log

import (
	"context"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *gpudLogger
var nopLogger = zap.NewNop().Sugar()

func init() {
	Logger = CreateLoggerWithConfig(DefaultLoggerConfig())
}

func DefaultLoggerConfig() *zap.Config {
	c := zap.NewProductionConfig()
	c.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return &c
}

func CreateLoggerWithLumberjack(logFile string, maxSize int, logLevel zapcore.Level) *gpudLogger {
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    maxSize, // megabytes
		MaxBackups: 5,
		MaxAge:     3,    // days
		Compress:   true, // compress the rotated files
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		w,
		logLevel,
	)
	logger := zap.New(core)
	return newGpudLogger(logger.Sugar())
}

func ParseLogLevel(logLevel string) (zap.AtomicLevel, error) {
	zapLvl := zap.NewAtomicLevel() // info level by default
	if logLevel != "" && logLevel != "info" {
		var err error
		zapLvl, err = zap.ParseAtomicLevel(logLevel)
		if err != nil {
			return zap.AtomicLevel{}, err
		}
	}
	return zapLvl, nil
}

func CreateLogger(logLevel zap.AtomicLevel, logFile string) *gpudLogger {
	if logFile != "" {
		return CreateLoggerWithLumberjack(logFile, 128, logLevel.Level())
	}

	lCfg := DefaultLoggerConfig()
	lCfg.Level = logLevel
	return CreateLoggerWithConfig(lCfg)
}

func CreateLoggerWithConfig(config *zap.Config) *gpudLogger {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	l, err := config.Build()
	if err != nil {
		panic(err)
	}

	return newGpudLogger(l.Sugar())
}

type gpudLogger struct {
	logger atomic.Pointer[zap.SugaredLogger]
}

func newGpudLogger(logger *zap.SugaredLogger) *gpudLogger {
	l := &gpudLogger{}
	l.set(logger)
	return l
}

func (l *gpudLogger) get() *zap.SugaredLogger {
	if l == nil {
		return nopLogger
	}
	logger := l.logger.Load()
	if logger == nil {
		return nopLogger
	}
	return logger
}

func (l *gpudLogger) set(logger *zap.SugaredLogger) {
	if logger == nil {
		logger = nopLogger
	}
	l.logger.Store(logger)
}

func SetLogger(logger *gpudLogger) {
	if logger == nil {
		Logger.set(nil)
		return
	}
	Logger.set(logger.get())
}

// Override the default logger's Errorw func to down level context canceled error
func (l *gpudLogger) Errorw(msg string, keysAndValues ...interface{}) {
	for i := 0; i < len(keysAndValues); i += 2 {
		if keysAndValues[i] != "error" {
			continue
		}
		if err, ok := keysAndValues[i+1].(error); ok {
			if strings.Contains(err.Error(), context.Canceled.Error()) {
				l.Warnw(msg, keysAndValues...)
				return
			}
		}
	}

	// Use underlying logger to avoid recursion into this wrapper.
	l.get().Errorw(msg, keysAndValues...) // nolint:staticcheck
}

// Implements "tailscale.com/types/logger".Logf.
func (l *gpudLogger) Printf(format string, v ...any) {
	l.Infof(format, v...)
}

func (l *gpudLogger) Debug(args ...interface{}) {
	l.get().Debug(args...)
}

func (l *gpudLogger) Debugf(template string, args ...interface{}) {
	l.get().Debugf(template, args...)
}

func (l *gpudLogger) Debugw(msg string, keysAndValues ...interface{}) {
	l.get().Debugw(msg, keysAndValues...)
}

func (l *gpudLogger) Info(args ...interface{}) {
	l.get().Info(args...)
}

func (l *gpudLogger) Infof(template string, args ...interface{}) {
	l.get().Infof(template, args...)
}

func (l *gpudLogger) Infow(msg string, keysAndValues ...interface{}) {
	l.get().Infow(msg, keysAndValues...)
}

func (l *gpudLogger) Warn(args ...interface{}) {
	l.get().Warn(args...)
}

func (l *gpudLogger) Warnf(template string, args ...interface{}) {
	l.get().Warnf(template, args...)
}

func (l *gpudLogger) Warnw(msg string, keysAndValues ...interface{}) {
	l.get().Warnw(msg, keysAndValues...)
}

func (l *gpudLogger) Error(args ...interface{}) {
	l.get().Error(args...)
}

func (l *gpudLogger) Errorf(template string, args ...interface{}) {
	l.get().Errorf(template, args...)
}

func (l *gpudLogger) Fatal(args ...interface{}) {
	l.get().Fatal(args...)
}

func (l *gpudLogger) With(args ...interface{}) *zap.SugaredLogger {
	return l.get().With(args...)
}

func (l *gpudLogger) Desugar() *zap.Logger {
	return l.get().Desugar()
}
