// Package process provides the process runner implementation on the host.
package process

import (
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

	"github.com/leptonai/gpud/log"
)

type Process interface {
	// Returns the copy of the labels of the process.
	Labels() map[string]string

	// Starts the process but does not wait for it to exit.
	Start(ctx context.Context) error
	// Returns true if the process is started.
	Started() bool

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
	labels map[string]string

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

	pid         int32
	commandArgs []string
	envs        []string
	runBashFile *os.File

	outputFile       *os.File
	stdoutReadCloser io.ReadCloser
	stderrReadCloser io.ReadCloser

	restartConfig *RestartConfig
}

func New(opts ...OpOption) (Process, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var cmdArgs []string
	var bashFile *os.File
	if op.runAsBashScript {
		var err error
		bashFile, err = os.CreateTemp(os.TempDir(), "tmpbash*.bash")
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

	for _, args := range op.commandsToRun {
		if bashFile == nil {
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
	return &process{
		labels: op.labels,
		cmd:    nil,

		started: false,
		aborted: false,

		errc: make(chan error, errcBuffer),

		commandArgs: cmdArgs,
		envs:        op.envs,
		runBashFile: bashFile,
		outputFile:  op.outputFile,

		restartConfig: op.restartConfig,
	}, nil
}

func (p *process) Labels() map[string]string {
	copied := make(map[string]string)
	for k, v := range p.labels {
		copied[k] = v
	}
	return copied
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

func (p *process) startCommand() error {
	log.Logger.Debugw("starting command", "command", p.commandArgs)
	p.cmd = exec.CommandContext(p.ctx, p.commandArgs[0], p.commandArgs[1:]...)
	p.cmd.Env = p.envs

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
				if exitErr.ExitCode() == -1 {
					if p.ctx.Err() != nil {
						log.Logger.Debugw("command was terminated (exit code -1) by the root context cancellation", "cmd", p.cmd.String(), "contextError", p.ctx.Err())
					} else {
						log.Logger.Warnw("command was terminated (exit code -1) for unknown reasons", "cmd", p.cmd.String())
					}
				} else {
					log.Logger.Warnw("command exited with non-zero status", "error", err, "cmd", p.cmd.String(), "exitCode", exitErr.ExitCode())
				}
			} else {
				log.Logger.Warnw("error waiting for command to finish", "error", err, "cmd", p.cmd.String())
			}

			if p.restartConfig == nil || !p.restartConfig.OnError {
				log.Logger.Warnw("process exited with error", "error", err)
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
		finished := false
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			if err.Error() == "os: process already finished" {
				finished = true
			} else {
				log.Logger.Warnw("failed to send SIGTERM to process", "error", err)
			}
		}
		if !finished {
			select {
			case <-p.ctx.Done():
			case <-time.After(3 * time.Second):
				if err := p.cmd.Process.Kill(); err != nil {
					log.Logger.Warnw("failed to send SIGKILL to process", "error", err)
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
