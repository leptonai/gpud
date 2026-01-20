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
	// Optional OpOption arguments can be passed to customize process behavior
	// (e.g., WithAllowDetachedProcess(true) for scripts with backgrounded commands).
	RunUntilCompletion(ctx context.Context, script string, opts ...OpOption) ([]byte, int32, error)
}
