// Package log provides the logging functionality for gpud.
package log

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger *LeptonLogger
)

func init() {
	Logger = CreateLoggerWithConfig(DefaultLoggerConfig())
}

func DefaultLoggerConfig() *zap.Config {
	c := zap.NewProductionConfig()
	c.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	return &c
}

func CreateLoggerWithLumberjack(logFile string, maxSize int, logLevel zapcore.Level) *LeptonLogger {
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    maxSize, // megabytes
		MaxBackups: 5,
		MaxAge:     3,    // days
		Compress:   true, // compress the rotated files
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		w,
		logLevel,
	)
	logger := zap.New(core)

	return &LeptonLogger{logger.Sugar()}
}

func ParseLogLevel(logLevel string) (zap.AtomicLevel, error) {
	var zapLvl zap.AtomicLevel = zap.NewAtomicLevel() // info level by default
	if logLevel != "" && logLevel != "info" {
		var err error
		zapLvl, err = zap.ParseAtomicLevel(logLevel)
		if err != nil {
			return zap.AtomicLevel{}, err
		}
	}
	return zapLvl, nil
}

func CreateLogger(logLevel zap.AtomicLevel, logFile string) *LeptonLogger {
	if logFile != "" {
		return CreateLoggerWithLumberjack(logFile, 128, logLevel.Level())
	}

	lCfg := DefaultLoggerConfig()
	lCfg.Level = logLevel
	return CreateLoggerWithConfig(lCfg)
}

func CreateLoggerWithConfig(config *zap.Config) *LeptonLogger {
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

// Implements "tailscale.com/types/logger".Logf.
func (l *LeptonLogger) Printf(format string, v ...any) {
	l.SugaredLogger.Infof(format, v...)
}
