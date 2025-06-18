package log

import (
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// AuditLog represents an GPUd audit log entry.
// c.f., https://pkg.go.dev/k8s.io/apiserver/pkg/apis/audit#Event
type AuditLog struct {
	Kind       string `json:"kind"`
	AuditID    string `json:"auditID"`
	MachineID  string `json:"machineID"`
	Stage      string `json:"stage"`
	RequestURI string `json:"requestURI"`
	Verb       string `json:"verb"`
	Data       any    `json:"data"`
}

type AuditOption func(*AuditLog)

func (op *AuditLog) applyOpts(opts []AuditOption) {
	for _, opt := range opts {
		opt(op)
	}

	if op.Kind == "" {
		op.Kind = "Event"
	}
	if op.AuditID == "" {
		op.AuditID = uuid.New().String()
	}
}

func WithKind(kind string) AuditOption {
	return func(ev *AuditLog) {
		ev.Kind = kind
	}
}

func WithAuditID(auditID string) AuditOption {
	return func(ev *AuditLog) {
		ev.AuditID = auditID
	}
}

func WithMachineID(machineID string) AuditOption {
	return func(ev *AuditLog) {
		ev.MachineID = machineID
	}
}

func WithStage(stage string) AuditOption {
	return func(ev *AuditLog) {
		ev.Stage = stage
	}
}

func WithRequestURI(requestURI string) AuditOption {
	return func(ev *AuditLog) {
		ev.RequestURI = requestURI
	}
}

func WithVerb(verb string) AuditOption {
	return func(ev *AuditLog) {
		ev.Verb = verb
	}
}

func WithData(data any) AuditOption {
	return func(ev *AuditLog) {
		ev.Data = data
	}
}

type AuditLogger interface {
	Log(...AuditOption)
}

func NewNopAuditLogger() AuditLogger {
	return &auditLogger{logger: zap.NewNop()}
}

func NewAuditLogger(logFile string) AuditLogger {
	var w zapcore.WriteSyncer
	if logFile != "" {
		w = zapcore.AddSync(&lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    128, // megabytes
			MaxBackups: 5,
			MaxAge:     3,    // days
			Compress:   true, // compress the rotated files
		})
	} else {
		w = zapcore.AddSync(os.Stdout)
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.LevelKey = ""
	encoderConfig.MessageKey = ""
	encoderConfig.CallerKey = ""
	encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339Nano)

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		w,
		zap.NewAtomicLevelAt(zap.InfoLevel),
	)
	logger := zap.New(core)

	return &auditLogger{logger: logger}
}

type auditLogger struct {
	logger *zap.Logger
}

func (l *auditLogger) Log(opts ...AuditOption) {
	ev := &AuditLog{}
	ev.applyOpts(opts)

	l.logger.Log(0, "",
		zap.String("kind", ev.Kind),
		zap.String("auditID", ev.AuditID),
		zap.String("machineID", ev.MachineID),
		zap.String("stage", ev.Stage),
		zap.String("requestURI", ev.RequestURI),
		zap.String("verb", ev.Verb),
		zap.Any("data", ev.Data),
	)
}

func CreateAuditLogFilepath(logFile string) string {
	return strings.ReplaceAll(logFile+".audit", ".log", "")
}
