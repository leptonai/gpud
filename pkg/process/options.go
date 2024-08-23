package process

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type OpOption func(*Op)

type Op struct {
	envs            []string
	outputFile      *os.File
	runAsBashScript bool

	restartConfig *RestartConfig
}

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	foundEnvs := make(map[string]any)
	for _, env := range op.envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable format: %s", env)
		}
		if _, ok := foundEnvs[parts[0]]; ok {
			return fmt.Errorf("duplicate environment variable: %s", parts[0])
		}
		foundEnvs[parts[0]] = parts[1]
	}

	if op.restartConfig != nil && op.restartConfig.Interval == 0 {
		op.restartConfig.Interval = 5 * time.Second
	}

	return nil
}

// Add a new environment variable to the process
// in the format of `KEY=VALUE`.
func WithEnvs(envs ...string) OpOption {
	return func(op *Op) {
		op.envs = append(op.envs, envs...)
	}
}

// Sets the file to which stderr and stdout will be written.
// For instance, you can set it to os.Stderr to pipe all the sub-process
// stderr and stdout to the parent process's stderr.
// Default is to set the os.Pipe to forward its output via io.ReadCloser.
func WithOutputFile(file *os.File) OpOption {
	return func(op *Op) {
		op.outputFile = file
	}
}

// Set true to run commands as a bash script.
// This is useful for running multiple/complicated commands.
func WithRunAsBashScript() OpOption {
	return func(op *Op) {
		op.runAsBashScript = true
	}
}

// Configures the process restart behavior.
func WithRestartConfig(config RestartConfig) OpOption {
	return func(op *Op) {
		op.restartConfig = &config
	}
}
