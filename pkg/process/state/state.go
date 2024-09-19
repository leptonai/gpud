// Package state provides a state management for processes
// such as the number of times a process has been started,
// and when it was last started, using the persistent storage.
package state

import (
	"context"

	"github.com/leptonai/gpud/pkg/process/state/schema"
)

type Interface interface {
	// RecordStart records the start of a script in UTC time.
	RecordStart(ctx context.Context, scriptHash string, opts ...OpOption) error
	// UpdateExitCode updates the exit code of a script.
	UpdateExitCode(ctx context.Context, scriptHash string, scriptExitCode int) error
	// UpdateOutput updates the output of a script.
	UpdateOutput(ctx context.Context, scriptHash string, scriptOutput string) error

	// Get gets the state of a script.
	// Returns row nil, error nil if the script hash does not exist.
	Get(ctx context.Context, scriptHash string) (*schema.Status, error)
}

type OpOption func(*Op)

type Op struct {
	ScriptName           string
	StartTimeUnixSeconds int64
}

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	return nil
}

func WithScriptName(scriptName string) OpOption {
	return func(op *Op) {
		op.ScriptName = scriptName
	}
}

func WithStartTimeUnixSeconds(startTimeUnixSeconds int64) OpOption {
	return func(op *Op) {
		op.StartTimeUnixSeconds = startTimeUnixSeconds
	}
}
