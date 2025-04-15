package process

import (
	"context"
	"errors"
)

var (
	ErrProcessAlreadyRunning = errors.New("process already running")
)

// Runner defines the interface for a process runner.
// It facillitates scheduling and running arbitrary bash scripts.
type Runner interface {
	// RunUntilCompletion starts a bash script, blocks until it finishes,
	// and returns the output and the exit code.
	// Whether to return an error when there is already a process running is up to the implementation.
	RunUntilCompletion(ctx context.Context, script string) ([]byte, int32, error)
}
