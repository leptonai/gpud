package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type OpOption func(*Op)

type Op struct {
	envs       []string
	outputFile *os.File

	commandsToRun           [][]string
	bashScriptContentsToRun string
	runAsBashScript         bool

	restartConfig *RestartConfig
}

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if len(op.commandsToRun) == 0 && op.bashScriptContentsToRun == "" {
		return errors.New("no command(s) or bash script contents provided")
	}
	if !op.runAsBashScript && len(op.commandsToRun) > 1 {
		return errors.New("cannot run multiple commands without a bash script mode")
	}
	for _, args := range op.commandsToRun {
		cmd := strings.Split(args[0], " ")[0]
		if !commandExists(cmd) {
			return fmt.Errorf("command not found: %q", cmd)
		}
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

	if op.bashScriptContentsToRun != "" && !op.runAsBashScript {
		op.runAsBashScript = true
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

// Add a new command to run.
func WithCommand(args ...string) OpOption {
	return func(op *Op) {
		op.commandsToRun = append(op.commandsToRun, args)
	}
}

// Sets/overwrites the commands to run.
func WithCommands(commands [][]string) OpOption {
	return func(op *Op) {
		op.commandsToRun = commands
	}
}

// Sets the bash script contents to run.
// This is useful for running multiple/complicated commands.
func WithBashScriptContentsToRun(script string) OpOption {
	return func(op *Op) {
		op.bashScriptContentsToRun = script
	}
}

// Sets the file to which stderr and stdout will be written.
// For instance, you can set it to os.Stderr to pipe all the sub-process
// stderr and stdout to the parent process's stderr.
// Default is to set the os.Pipe to forward its output via io.ReadCloser.
//
// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
// If retry configuration is specified, specify the output file to read all the output.
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
// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
func WithRestartConfig(config RestartConfig) OpOption {
	return func(op *Op) {
		op.restartConfig = &config
	}
}

func commandExists(name string) bool {
	p, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	return p != ""
}
