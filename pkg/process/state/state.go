// Package state provides a state management for processes
// such as the number of times a process has been started,
// and when it was last started, using the persistent storage.
package state

import (
	"context"

	"github.com/leptonai/gpud/pkg/process/state/schema"
)

type Interface interface {
	// RecordStart records the start of a script.
	RecordStart(ctx context.Context, scriptHash string, scriptName string) error
	// UpdateExitCode updates the exit code of a script.
	UpdateExitCode(ctx context.Context, scriptHash string, scriptExitCode int) error
	// UpdateOutput updates the output of a script.
	UpdateOutput(ctx context.Context, scriptHash string, scriptOutput string) error
	// Get gets the state of a script.
	Get(ctx context.Context, scriptHash string) (*schema.Row, error)
}
