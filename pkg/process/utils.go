package process

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
)

type ReadOpOption func(*ReadOp)

type ReadOp struct {
	readStdout bool
	readStderr bool

	processLine func(line string)
	waitForCmd  bool
}

func (op *ReadOp) applyOpts(opts []ReadOpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.processLine == nil {
		op.processLine = func(line string) {
			// no-op
		}
	}

	if !op.readStdout && !op.readStderr {
		return errors.New("at least one of readStdout or readStderr must be true")
	}

	return nil
}

func WithReadStdout() ReadOpOption {
	return func(op *ReadOp) {
		op.readStdout = true
	}
}

func WithReadStderr() ReadOpOption {
	return func(op *ReadOp) {
		op.readStderr = true
	}
}

// Sets a function to process each line of the command output.
// Helps with debugging if command times out in the middle of reading.
func WithProcessLine(fn func(line string)) ReadOpOption {
	return func(op *ReadOp) {
		op.processLine = fn
	}
}

func WithWaitForCmd() ReadOpOption {
	return func(op *ReadOp) {
		op.waitForCmd = true
	}
}

var (
	ErrProcessNotStarted = errors.New("process not started")
	ErrProcessAborted    = errors.New("process aborted")
)

func ReadAll(ctx context.Context, p Process) ([]byte, error) {
	if !p.Started() {
		return nil, ErrProcessNotStarted
	}
	if p.Closed() {
		return nil, ErrProcessAborted
	}

	rd := io.MultiReader(p.StdoutReader(), p.StderrReader())
	b, err := io.ReadAll(rd)
	if err != nil {
		if !strings.Contains(err.Error(), "file already closed") {
			return nil, err
		}
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			return b, err
		}
	case <-ctx.Done():
		return b, ctx.Err()
	}

	return b, nil
}

func Read(ctx context.Context, p Process, opts ...ReadOpOption) error {
	if !p.Started() {
		return ErrProcessNotStarted
	}
	if p.Closed() {
		return ErrProcessAborted
	}

	op := &ReadOp{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	// combine stdout and stderr into a single reader
	readers := []io.Reader{}
	if op.readStdout {
		// may happen if the process is alread aborted
		stdoutReader := p.StdoutReader()
		if stdoutReader == nil {
			return errors.New("stdout reader is nil")
		}
		readers = append(readers, stdoutReader)
	}
	if op.readStderr {
		// may happen if the process is alread aborted
		stderrReader := p.StderrReader()
		if stderrReader == nil {
			return errors.New("stderr reader is nil")
		}
		readers = append(readers, stderrReader)
	}

	combinedReader := io.MultiReader(readers...)
	scanner := bufio.NewScanner(combinedReader)
	if scanner == nil {
		return errors.New("scanner is nil")
	}

	for scanner.Scan() {
		// helps with debugging if command times out in the middle of reading
		op.processLine(scanner.Text())

		select {
		case err := <-p.Wait(): // command failed
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if p.Closed() {
			return errors.New("process aborted")
		}
	}

	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return serr
		}
	}

	if op.waitForCmd {
		select {
		case err := <-p.Wait():
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}
