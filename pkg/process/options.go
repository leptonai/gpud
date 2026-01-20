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
	labels map[string]string

	envs       []string
	outputFile *os.File

	commandsToRun           [][]string
	bashScriptContentsToRun string

	runAsBashScript bool

	// runBashInline executes the bash contents via `bash -c` without writing a temp file.
	runBashInline bool

	// temporary directory to store bash script files
	bashScriptTmpDirectory string
	// pattern of the bash script file names
	// e.g., "tmpbash*.bash"
	bashScriptFilePattern string

	restartConfig *RestartConfig

	// waitForDetach specifies a grace period to wait in Close() before killing
	// the process group. This is useful for commands that spawn background processes
	// that should outlive the parent, such as:
	//
	//   sleep 10 && systemctl restart gpud &
	//
	// In this pattern, the bash script exits immediately (after backgrounding the command),
	// but the backgrounded "sleep 10 && systemctl restart gpud" should continue running.
	// Without a grace period, Close() would immediately kill the entire process group,
	// preventing the delayed restart from occurring.
	//
	// With WithWaitForDetach(15*time.Second), Close() will wait up to 15 seconds
	// for the backgrounded process to complete before sending kill signals.
	waitForDetach time.Duration
}

const DefaultBashScriptFilePattern = "gpud-*.bash"

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.labels == nil {
		op.labels = make(map[string]string)
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

	if op.bashScriptTmpDirectory == "" {
		op.bashScriptTmpDirectory = os.TempDir()
	}

	if op.bashScriptFilePattern == "" {
		op.bashScriptFilePattern = DefaultBashScriptFilePattern
	}

	return nil
}

func WithLabel(key, value string) OpOption {
	return func(op *Op) {
		if op.labels == nil {
			op.labels = make(map[string]string)
		}
		op.labels[key] = value
	}
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

// WithRunBashInline executes the bash script without creating a temp file.
// Implementation note:
//   - We prefer `bash -s` and feed the script via stdin (instead of `bash -c`).
//     Why `-s` over `-c` across macOS and Linux:
//   - No ARG_MAX issues: `-s` reads the script from stdin; `-c` places the entire
//     script into an argv element, which can hit kernel argument-length limits.
//   - Fewer quoting pitfalls: `-c` requires double-quoting/escaping the whole
//     script; stdin avoids extra quoting layers and shell metacharacter surprises.
//   - Same semantics on macOS and Linux: both support `-s` and `-c` consistently;
//     stdin is the most portable way for inline scripts.
//
// Default remains file-backed (WithRunAsBashScript without inline) for backwards
// compatibility; use this when disk writes are undesirable
// (e.g., "no space left on device").
func WithRunBashInline() OpOption {
	return func(op *Op) {
		op.runAsBashScript = true
		op.runBashInline = true
	}
}

// Sets the temporary directory to store bash script files.
// Default is to use the system's temporary directory.
func WithBashScriptTmpDirectory(dir string) OpOption {
	return func(op *Op) {
		op.bashScriptTmpDirectory = dir
	}
}

// Sets the pattern of the bash script file names.
// Default is to use "tmpbash*.bash".
func WithBashScriptFilePattern(pattern string) OpOption {
	return func(op *Op) {
		op.bashScriptFilePattern = pattern
	}
}

// Configures the process restart behavior.
// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
func WithRestartConfig(config RestartConfig) OpOption {
	return func(op *Op) {
		op.restartConfig = &config
	}
}

// WithWaitForDetach sets a grace period to wait in Close() before killing
// the process group. This is essential for commands that spawn background
// processes intended to outlive the parent shell.
//
// USE CASE - Delayed Service Restart:
//
//	sleep 10 && systemctl restart gpud &
//
// This pattern is common in deployment scripts where gpud needs to be restarted
// after a delay (e.g., to allow installation scripts to complete). The "&" causes
// bash to background the command and exit immediately.
//
// THE PROBLEM:
// When using process groups (Setpgid=true), the backgrounded command shares the
// same Process Group ID (PGID) as the parent bash. When Close() is called, it
// sends SIGKILL to -PGID, killing ALL processes in the group - including the
// backgrounded "sleep 10 && systemctl restart gpud" that should continue running.
//
// THE SOLUTION:
// With WithWaitForDetach(15*time.Second), Close() will wait up to 15 seconds
// for all processes in the group to complete before sending kill signals.
// If the backgrounded process completes within the grace period (e.g., after
// the 10-second sleep and restart), no kill signals are sent.
//
// Example:
//
//	p, err := New(
//	    WithBashScriptContentsToRun(deployScript),
//	    WithWaitForDetach(15*time.Second),
//	)
func WithWaitForDetach(d time.Duration) OpOption {
	return func(op *Op) {
		op.waitForDetach = d
	}
}

func commandExists(name string) bool {
	p, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	return p != ""
}
