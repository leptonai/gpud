// Package process provides the process runner implementation on the host.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

type Process interface {
	// Starts the process but does not wait for it to exit.
	Start(ctx context.Context) error
	// Returns true if the process is started.
	Started() bool

	// StartAndWaitForCombinedOutput starts the process and returns the combined output of the command.
	// Returns ErrProcessAlreadyStarted if the process is already started.
	StartAndWaitForCombinedOutput(ctx context.Context) ([]byte, error)

	// Closes the process (aborts if still running) and waits for it to exit.
	// Cleans up the process resources.
	Close(ctx context.Context) error
	// Returns true if the process is closed.
	Closed() bool

	// Waits for the process to exit and returns the error, if any.
	// If the command completes successfully, the error will be nil.
	Wait() <-chan error

	// Returns the current pid of the process.
	PID() int32

	// Returns the exit code of the process.
	// Returns 0 if the process is not started yet.
	// Returns non-zero if the process exited with a non-zero exit code.
	ExitCode() int32

	// Returns the stdout reader.
	// stderr/stdout piping sometimes doesn't work well on latest mac with io.ReadAll
	// Use bufio.NewScanner(p.StdoutReader()) instead.
	//
	// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
	// If retry configuration is specified, specify the output file to read all the output.
	//
	// The returned reader is set to nil upon the abort call on the process,
	// to prevent redundant closing of the reader.
	StdoutReader() io.Reader

	// Returns the stderr reader.
	// stderr/stdout piping sometimes doesn't work well on latest mac with io.ReadAll
	// Use bufio.NewScanner(p.StderrReader()) instead.
	//
	// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
	// If retry configuration is specified, specify the output file to read all the output.
	//
	// The returned reader is set to nil upon the abort call on the process,
	// to prevent redundant closing of the reader.
	StderrReader() io.Reader
}

// RestartConfig is the configuration for the process restart.
// If the process exits with a non-zero exit code, stdout/stderr pipes may not work.
// If retry configuration is specified, specify the output file to read all the output.
type RestartConfig struct {
	// Set true to restart the process on error exit.
	OnError bool
	// Set the maximum number of restarts.
	Limit int
	// Set the interval between restarts.
	Interval time.Duration
}

type process struct {
	ctx    context.Context
	cancel context.CancelFunc

	cmdMu sync.RWMutex
	cmd   *exec.Cmd

	startedMu sync.RWMutex
	started   bool

	abortedMu sync.RWMutex
	aborted   bool

	// error streaming channel, closed on command exit
	errc chan error

	pid      int32
	exitCode int32

	commandArgs []string
	envs        []string
	runBashFile *os.File

	outputFile       *os.File
	stdoutReadCloser io.ReadCloser
	stderrReadCloser io.ReadCloser

	restartConfig *RestartConfig

	// input bytes to feed to the command's stdin
	getStdinBytesReader func() *bytes.Reader
}

func New(opts ...OpOption) (Process, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var cmdArgs []string
	var bashFile *os.File
	if op.runAsBashScript {
		// Option 1: inline bash, avoid writing any file to disk
		if op.runBashInline {
			cmdArgs = []string{"bash", "-s"}
			// feed the script over stdin to avoid ARG_MAX limits and quoting hassles
			// store in process to wire during startCommand
			// set later in the returned process struct
			// (we assign below when constructing the process)
		} else {
			// Option 2: file-backed bash, write to a temp file on disk
			// may fail if the disk is full
			// e.g.,
			// "open /tmp/gpud-453112547.bash: no space left on device"
			var err error
			bashFile, err = os.CreateTemp(op.bashScriptTmpDirectory, op.bashScriptFilePattern)
			if err != nil {
				return nil, err
			}

			if op.bashScriptContentsToRun != "" { // assume the bash script provided by the user is a complete script
				if _, err := bashFile.Write([]byte(op.bashScriptContentsToRun)); err != nil {
					return nil, err
				}
			} else {
				if _, err := bashFile.Write([]byte(bashScriptHeader)); err != nil {
					return nil, err
				}
			}
			defer func() {
				_ = bashFile.Sync()
			}()
			cmdArgs = []string{"bash", bashFile.Name()}
		}
	}

	for _, args := range op.commandsToRun {
		if op.runBashInline { // inline script already assembled above
			continue
		}
		if bashFile == nil { // non-bash mode: single command
			cmdArgs = args
			continue
		}

		if _, err := bashFile.Write([]byte(strings.Join(args, " "))); err != nil {
			return nil, err
		}
		if _, err := bashFile.Write([]byte("\n")); err != nil {
			return nil, err
		}
	}

	errcBuffer := 1
	if op.restartConfig != nil && op.restartConfig.OnError && op.restartConfig.Limit > 0 {
		errcBuffer = op.restartConfig.Limit
	}
	proc := &process{
		cmd: nil,

		started: false,
		aborted: false,

		errc: make(chan error, errcBuffer),

		commandArgs: cmdArgs,
		envs:        op.envs,
		runBashFile: bashFile,
		outputFile:  op.outputFile,

		restartConfig: op.restartConfig,
	}

	if op.runAsBashScript && op.runBashInline {
		if op.bashScriptContentsToRun != "" {
			proc.getStdinBytesReader = func() *bytes.Reader {
				return bytes.NewReader([]byte(op.bashScriptContentsToRun))
			}
		} else {
			var b bytes.Buffer
			b.WriteString(bashScriptHeader)
			for _, args := range op.commandsToRun {
				b.WriteString(strings.Join(args, " "))
				b.WriteByte('\n')
			}
			proc.getStdinBytesReader = func() *bytes.Reader {
				return bytes.NewReader(b.Bytes())
			}
		}
	}

	return proc, nil
}

func (p *process) Start(ctx context.Context) error {
	p.startedMu.RLock()
	started := p.started
	p.startedMu.RUnlock()
	if started { // already started
		return nil
	}

	p.abortedMu.RLock()
	aborted := p.aborted
	p.abortedMu.RUnlock()
	if aborted { // already aborted
		return nil
	}

	p.cmdMu.Lock()
	defer p.cmdMu.Unlock()

	if p.cmd != nil {
		return errors.New("process already started")
	}

	cctx, ccancel := context.WithCancel(ctx)
	p.ctx = cctx
	p.cancel = ccancel

	if err := p.startCommand(); err != nil {
		return err
	}

	go func() {
		p.watchCmd()
	}()

	return nil
}

func (p *process) Started() bool {
	p.startedMu.RLock()
	defer p.startedMu.RUnlock()

	return p.started
}

func (p *process) createCmd() *exec.Cmd {
	return exec.CommandContext(p.ctx, p.commandArgs[0], p.commandArgs[1:]...)
}

func (p *process) startCommand() error {
	log.Logger.Debugw("starting command", "command", p.commandArgs)
	p.cmd = p.createCmd()
	if p.getStdinBytesReader != nil {
		p.cmd.Stdin = p.getStdinBytesReader()
	}
	p.cmd.Env = p.envs

	// Create a new process group for this command and all its children.
	//
	// When running shell commands that spawn background processes (e.g., "cmd1 & cmd2 & wait"),
	// the shell creates child processes that are NOT automatically terminated when we kill the
	// parent shell. Without process groups:
	//   1. Go's Process.Signal() only sends signals to the direct child (the shell)
	//   2. Backgrounded processes (cmd1, cmd2) become orphaned and reparented to PID 1
	//   3. These orphaned processes continue running indefinitely (process leak)
	//
	// With Setpgid=true:
	//   1. The shell and ALL its children share the same Process Group ID (PGID)
	//   2. The PGID equals the shell's PID
	//   3. We can kill the entire group at once using syscall.Kill(-pgid, signal)
	//
	// EXAMPLE SCENARIO (fabric-manager log watcher):
	//   Command: "tail -f /var/log/fabricmanager.log & journalctl -u nvidia-fabricmanager -f & wait"
	//   Without Setpgid: Killing bash leaves tail and journalctl running forever
	//   With Setpgid: Killing -PGID terminates bash, tail, AND journalctl together
	//
	// NOTE: This is Unix/Linux specific. On Windows, process groups work differently.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Set a custom Cancel function to kill the entire process group when context is cancelled.
	// Without this, Go's default context cancellation (exec.CommandContext) only calls
	// os.Process.Kill() on the parent process, leaving any backgrounded child processes
	// as orphans (reparented to PID 1) that continue running indefinitely.
	//
	// By killing the negative PGID (-pgid), we send SIGKILL to ALL processes in the group,
	// ensuring consistent cleanup behavior with the Close() method.
	p.cmd.Cancel = func() error {
		if p.cmd.Process == nil {
			return nil
		}
		pgid := p.cmd.Process.Pid
		// Use SIGKILL for context cancellation since we want immediate termination.
		// ESRCH ("no such process") is expected if the group already exited.
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}

	switch {
	case p.outputFile != nil:
		p.cmd.Stdout = p.outputFile
		p.cmd.Stderr = p.outputFile

		p.stdoutReadCloser = p.outputFile
		p.stderrReadCloser = p.outputFile

	default:
		var err error
		p.stdoutReadCloser, err = p.cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe: %w", err)
		}
		p.stderrReadCloser, err = p.cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to get stderr pipe: %w", err)
		}
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}
	atomic.StoreInt32(&p.pid, int32(p.cmd.Process.Pid))

	p.startedMu.Lock()
	p.started = true
	p.startedMu.Unlock()

	return nil
}

var ErrProcessAlreadyStarted = errors.New("process already started")

func (p *process) StartAndWaitForCombinedOutput(ctx context.Context) ([]byte, error) {
	if p.Started() {
		return nil, ErrProcessAlreadyStarted
	}

	cctx, ccancel := context.WithCancel(ctx)
	p.ctx = cctx
	p.cancel = ccancel

	p.cmdMu.Lock()
	defer p.cmdMu.Unlock()

	p.cmd = p.createCmd()
	if p.getStdinBytesReader != nil {
		p.cmd.Stdin = p.getStdinBytesReader()
	}
	p.cmd.Env = p.envs

	// Use process groups for consistent behavior with Start().
	// See detailed comments in startCommand() for why this is necessary.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Set a custom Cancel function to kill the entire process group when context is cancelled.
	// Without this, Go's default context cancellation (exec.CommandContext) only calls
	// os.Process.Kill() on the parent process, leaving any backgrounded child processes
	// as orphans (reparented to PID 1) that continue running indefinitely.
	//
	// By killing the negative PGID (-pgid), we send SIGKILL to ALL processes in the group,
	// ensuring consistent cleanup behavior with the Close() method.
	p.cmd.Cancel = func() error {
		if p.cmd.Process == nil {
			return nil
		}
		pgid := p.cmd.Process.Pid
		// Use SIGKILL for context cancellation since we want immediate termination.
		// ESRCH ("no such process") is expected if the group already exited.
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}

	// ref. "os/exec" "CombinedOutput"
	b := bytes.NewBuffer(nil)
	p.cmd.Stdout = b
	p.cmd.Stderr = b
	if err := p.cmd.Start(); err != nil {
		// may fail from the command error (e.g., exit 255)
		// we still return the partial output
		return b.Bytes(), fmt.Errorf("failed to start command: %w", err)
	}
	atomic.StoreInt32(&p.pid, int32(p.cmd.Process.Pid))

	p.startedMu.Lock()
	p.started = true
	p.startedMu.Unlock()

	if err := p.cmd.Wait(); err != nil {
		// may fail from the command error (e.g., exit 255)
		// we still return the partial output
		return b.Bytes(), fmt.Errorf("command exited with error: %w", err)
	}

	return b.Bytes(), nil
}

// Returns a channel where the command watcher sends the error if any.
// The channel is closed on the command exit.
func (p *process) Wait() <-chan error {
	return p.errc
}

func (p *process) watchCmd() {
	if p.cmd == nil {
		return
	}
	defer func() {
		close(p.errc)
	}()

	restartCount := 0
	for {
		if p.cmd.Process == nil { // Wait cannot be called if the process is not started yet
			select {
			case <-p.ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		errc := make(chan error)
		go func() {
			errc <- p.cmd.Wait()
		}()

		select {
		case <-p.ctx.Done():
			// command aborted (e.g., Stop called)
			// cmd.Wait will return error
			err := <-errc
			p.errc <- err
			return

		case err := <-errc:
			p.errc <- err

			if err == nil {
				log.Logger.Debugw("process exited successfully")
				return
			}

			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode := exitErr.ExitCode()
				atomic.StoreInt32(&p.exitCode, int32(exitCode))

				if exitCode == -1 {
					if p.ctx.Err() != nil {
						log.Logger.Debugw("command was terminated (exit code -1) by the root context cancellation", "cmd", p.cmd.String(), "contextError", p.ctx.Err())
					} else {
						log.Logger.Warnw("command was terminated (exit code -1) for unknown reasons", "cmd", p.cmd.String())
					}
				} else {
					log.Logger.Debugw("command exited with non-zero status", "error", err, "cmd", p.cmd.String(), "exitCode", exitCode)
				}
			} else {
				log.Logger.Warnw("error waiting for command to finish", "error", err, "cmd", p.cmd.String())
			}

			if p.restartConfig == nil || !p.restartConfig.OnError {
				log.Logger.Debugw("process exited with error", "error", err)
				return
			}

			if p.restartConfig.Limit > 0 && restartCount >= p.restartConfig.Limit {
				log.Logger.Warnw("process exited with error, but restart limits reached", "restartCount", restartCount, "error", err)
				return
			}
		}

		select {
		case <-p.ctx.Done():
			return
		case <-time.After(p.restartConfig.Interval):
		}

		if err := p.startCommand(); err != nil {
			log.Logger.Warnw("failed to restart command", "error", err)
			return
		}

		restartCount++
	}
}

func (p *process) Close(ctx context.Context) error {
	p.startedMu.RLock()
	started := p.started
	p.startedMu.RUnlock()
	if !started { // has not started yet
		return nil
	}

	p.abortedMu.RLock()
	aborted := p.aborted
	p.abortedMu.RUnlock()
	if aborted { // already aborted
		return nil
	}

	p.cmdMu.Lock()
	defer p.cmdMu.Unlock()

	if p.cmd == nil {
		return errors.New("process not started")
	}

	p.cancel()

	if p.cmd.Process != nil {
		// Kill the entire process group, not just the parent process.
		//
		// WHY WE USE NEGATIVE PID:
		// When we set Setpgid=true in startCommand(), the process and all its children
		// share the same Process Group ID (PGID), which equals the parent's PID.
		// Using syscall.Kill with a NEGATIVE pid sends the signal to every process
		// in that group:
		//   - syscall.Kill(pid, signal)  -> kills only the process with that PID
		//   - syscall.Kill(-pid, signal) -> kills ALL processes in the group with PGID=pid
		//
		// WHY THIS MATTERS:
		// Consider a command like: "tail -f file & journalctl -f & wait"
		//   - Parent bash shell (PID=1000, PGID=1000)
		//   - Child tail process (PID=1001, PGID=1000)  <- same group!
		//   - Child journalctl process (PID=1002, PGID=1000)  <- same group!
		//
		// Without negative PID: Only bash (1000) receives SIGTERM
		//   -> tail and journalctl become orphans, reparented to init (PID 1)
		//   -> They keep running forever (PROCESS LEAK!)
		//
		// With negative PID: syscall.Kill(-1000, SIGTERM) sends to ALL three processes
		//   -> bash, tail, AND journalctl all receive SIGTERM
		//   -> Clean shutdown with no orphaned processes
		//
		// NOTE: If the process group no longer exists (already exited), Kill returns
		// ESRCH ("no such process") which we handle gracefully.
		pgid := p.cmd.Process.Pid
		finished := false
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
			if err == syscall.ESRCH {
				// ESRCH means "no such process" - the process group already exited
				finished = true
			} else {
				log.Logger.Warnw("failed to send SIGTERM to process group", "pgid", pgid, "error", err)
			}
		}
		if !finished {
			select {
			case <-p.ctx.Done():
			case <-time.After(3 * time.Second):
				// Process group didn't exit gracefully within 3 seconds, force kill
				if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
					// Don't warn on ESRCH - process may have exited between SIGTERM and SIGKILL
					if err != syscall.ESRCH {
						log.Logger.Warnw("failed to send SIGKILL to process group", "pgid", pgid, "error", err)
					}
				}
			}
		}
	}

	if p.runBashFile != nil {
		_ = p.runBashFile.Sync()
		_ = p.runBashFile.Close()
		if err := os.RemoveAll(p.runBashFile.Name()); err != nil {
			log.Logger.Warnw("failed to remove bash file", "error", err)
			// Don't return here, continue with cleanup
		}
	}

	if p.stdoutReadCloser != nil {
		_ = p.stdoutReadCloser.Close()

		// set to nil to prevent redundant closing of the reader
		p.stdoutReadCloser = nil
	}

	if p.stderrReadCloser != nil {
		_ = p.stderrReadCloser.Close()

		// set to nil to prevent redundant closing of the reader
		p.stderrReadCloser = nil
	}

	if p.cmd.Cancel != nil { // if created with CommandContext
		_ = p.cmd.Cancel()
	}

	// do not set p.cmd to nil
	// as Wait is still waiting for the process to exit
	// p.cmd = nil

	p.abortedMu.Lock()
	p.aborted = true
	p.abortedMu.Unlock()

	return nil
}

func (p *process) Closed() bool {
	p.abortedMu.RLock()
	defer p.abortedMu.RUnlock()

	return p.aborted
}

func (p *process) PID() int32 {
	return atomic.LoadInt32(&p.pid)
}

func (p *process) ExitCode() int32 {
	return atomic.LoadInt32(&p.exitCode)
}

func (p *process) StdoutReader() io.Reader {
	p.cmdMu.RLock()
	defer p.cmdMu.RUnlock()

	if p.outputFile != nil {
		return p.outputFile
	}
	return p.stdoutReadCloser
}

func (p *process) StderrReader() io.Reader {
	p.cmdMu.RLock()
	defer p.cmdMu.RUnlock()

	if p.outputFile != nil {
		return p.outputFile
	}
	return p.stderrReadCloser
}

const bashScriptHeader = `#!/bin/bash

# do not mask errors in a pipeline
set -o pipefail

# treat unset variables as an error
set -o nounset

# exit script whenever it errs
set -o errexit

`
