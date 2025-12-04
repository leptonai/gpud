package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

var _ Runner = &exclusiveRunner{}

func NewExclusiveRunner() Runner {
	return &exclusiveRunner{}
}

// exclusiveRunner is a scheduler that runs a single process at a time.
// Does not support concurrent execution of multiple processes.
type exclusiveRunner struct {
	mu      sync.RWMutex
	running Process
}

var defaultScriptsDir = filepath.Join(os.TempDir(), "gpud-scripts-runner")

// RunUntilCompletion starts a bash script, blocks until it finishes,
// and returns the output and the exit code.
// If there is already a process running, it returns an error.
func (er *exclusiveRunner) RunUntilCompletion(ctx context.Context, script string) ([]byte, int32, error) {
	if er.alreadyRunning() {
		return nil, 0, ErrProcessAlreadyRunning
	}

	// write all stderr + stdout to a temporary file
	if err := os.MkdirAll(defaultScriptsDir, 0755); err != nil {
		return nil, 0, fmt.Errorf("failed to create temp dir %s: %w", defaultScriptsDir, err)
	}
	tmpFile, err := os.CreateTemp(defaultScriptsDir, "gpud-process-output-*.txt")
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	defer func() {
		_ = tmpFile.Close()
	}()

	p, err := New(
		WithBashScriptContentsToRun(script),
		WithOutputFile(tmpFile),
	)
	if err != nil {
		return nil, 0, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Errorw("failed to close process", "pid", p.PID(), "error", err)
		}
		log.Logger.Debugw("closed running script", "pid", p.PID())
		er.mu.Lock()
		er.running = nil
		er.mu.Unlock()
	}()
	log.Logger.Debugw("started running script", "pid", p.PID())

	er.mu.Lock()
	er.running = p
	er.mu.Unlock()

	select {
	case <-ctx.Done():
		log.Logger.Warnw("process aborted before completion", "pid", p.PID())
		return nil, p.ExitCode(), ctx.Err()

	case err := <-p.Wait():
		if err != nil {
			// even if the command failed and aborted in the middle with non-zero exit code,
			// we still want to return the partial output
			// in case the output parser is configured
			output, rerr := os.ReadFile(tmpFile.Name())
			if rerr != nil {
				log.Logger.Errorw("failed to read output file after the process failed", "error", rerr)
			}
			if len(output) == 0 {
				output = nil
			}

			return output, p.ExitCode(), err
		}
		log.Logger.Debugw("process exited", "pid", p.PID(), "exitCode", p.ExitCode())
	}

	output, err := os.ReadFile(tmpFile.Name())
	return output, p.ExitCode(), err
}

func (er *exclusiveRunner) alreadyRunning() bool {
	er.mu.RLock()
	defer er.mu.RUnlock()

	return er.running != nil
}
