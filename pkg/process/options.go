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

	// allowDetachedProcess controls whether backgrounded processes can outlive the parent.
	// When true, Setpgid is NOT used, allowing backgrounded commands like
	// "sleep 10 && systemctl restart gpud &" to continue running after the parent exits.
	// When false (default), Setpgid is used to create a process group, and Close()
	// will kill all processes in the group together - this is safer and prevents
	// orphaned processes but prevents the "&" background pattern from working.
	allowDetachedProcess bool
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

// WithAllowDetachedProcess controls whether backgrounded processes can outlive the parent shell.
//
// When allow=true:
//   - Setpgid is NOT set (processes run in parent's process group)
//   - Only the direct child process (shell) is killed on Close()
//   - Backgrounded processes (using "&") become orphans and continue running
//   - USE THIS for scripts that end with patterns like: "sleep 10 && systemctl restart gpud &"
//
// When allow=false (DEFAULT):
//   - Setpgid is set (creates a new process group)
//   - Close() kills the entire process group (parent AND all children)
//   - Safer behavior that prevents orphaned/leaked processes
//   - USE THIS for normal commands where you want clean process cleanup
//
// Example use case for allow=true:
//
//	Package installation scripts often end with:
//	  sleep 10 && systemctl restart gpud &
//	This schedules a delayed restart of gpud after the script exits.
//	Without allowDetachedProcess=true, the backgrounded command would be killed
//	when Close() is called.
//
// Example:
//
//	p, err := New(
//	    WithBashScriptContentsToRun(deployScript),
//	    WithAllowDetachedProcess(true),
//	)
func WithAllowDetachedProcess(allow bool) OpOption {
	return func(op *Op) {
		op.allowDetachedProcess = allow
	}
}

func commandExists(name string) bool {
	p, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	return p != ""
}
