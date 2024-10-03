// Package reboot provides a function to reboot the system.
package reboot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	stdos "os"
	"strings"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

type Op struct {
	useSystemctl bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

// Set true to run "systemctl reboot".
func WithSystemctl(b bool) OpOption {
	return func(op *Op) {
		op.useSystemctl = b
	}
}

var ErrNotRoot = errors.New("must be run as sudo/root")

// Reboots the system.
func Reboot(ctx context.Context, opts ...OpOption) error {
	options := &Op{}
	if err := options.applyOpts(opts); err != nil {
		return err
	}

	asRoot := stdos.Geteuid() == 0 // running as root
	if !asRoot {
		return ErrNotRoot
	}

	// "sudo shutdown -r +1" does not work
	cmd := "subo reboot"
	if options.useSystemctl {
		cmd = "sudo systemctl reboot"
	}

	proc, err := process.New(
		process.WithCommand(cmd),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return err
	}

	log.Logger.Warnw("rebooting", "command", cmd)
	if err := proc.Start(ctx); err != nil {
		return err
	}
	// actually, this should not print if reboot worked
	log.Logger.Infow("successfully rebooted", "command", cmd)

	scanner := bufio.NewScanner(proc.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		fmt.Println("stdout:", line)
		select {
		case err := <-proc.Wait():
			if err != nil {
				return err
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			panic(serr)
		}
	}

	return nil
}
