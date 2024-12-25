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

func Read(ctx context.Context, p Process, opts ...ReadOpOption) error {
	op := &ReadOp{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	// combine stdout and stderr into a single reader
	readers := []io.Reader{}
	if op.readStdout {
		readers = append(readers, p.StdoutReader())
	}
	if op.readStderr {
		readers = append(readers, p.StderrReader())
	}

	combinedReader := io.MultiReader(readers...)
	scanner := bufio.NewScanner(combinedReader)

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
