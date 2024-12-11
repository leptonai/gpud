package process

import (
	"bufio"
	"context"
	"strings"
)

type ReadOpOption func(*ReadOp)

type ReadOp struct {
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

	return nil
}

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

func ReadAllStdout(ctx context.Context, p Process, opts ...ReadOpOption) error {
	op := &ReadOp{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	scanner := bufio.NewScanner(p.StdoutReader())

	for scanner.Scan() {
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
