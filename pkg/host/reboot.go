package host

import (
	"context"
	"errors"
	"fmt"
	stdos "os"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type Op struct {
	delaySeconds int
	useSystemctl bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

// Specifies the delay seconds before rebooting.
// Useful for the remote server to get the response from a GPUd daemon.
func WithDelaySeconds(delaySeconds int) OpOption {
	return func(op *Op) {
		op.delaySeconds = delaySeconds
	}
}

// Set true to run "systemctl reboot".
func WithSystemctl(b bool) OpOption {
	return func(op *Op) {
		op.useSystemctl = b
	}
}

var ErrNotRoot = errors.New("must be run as sudo/root")

// Reboot reboots the system.
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
	cmd := "sudo reboot"
	if options.useSystemctl {
		cmd = "sudo systemctl reboot"
	}

	if options.delaySeconds == 0 {
		log.Logger.Infow("rebooting immediately", "command", cmd)
		return runReboot(ctx, cmd)
	}

	go func() {
		select {
		case <-time.After(time.Duration(options.delaySeconds) * time.Second):
			log.Logger.Infow("delay expired, rebooting now", "command", cmd)
		case <-ctx.Done():
			log.Logger.Warnw("context done, aborting reboot", "command", cmd)
			return
		}

		rerr := runReboot(ctx, cmd)

		// actually, this should not print if reboot worked
		log.Logger.Warnw("successfully rebooted", "command", cmd, "error", rerr)
	}()

	log.Logger.Infow(
		"triggering reboot after delay",
		"delaySeconds", options.delaySeconds,
		"command", cmd,
	)
	return nil
}

func runReboot(ctx context.Context, cmd string) error {
	proc, err := process.New(
		process.WithCommand(cmd),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
	)
	if err != nil {
		return err
	}

	if err := proc.Start(ctx); err != nil {
		return err
	}
	defer func() {
		if err := proc.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	if err := process.Read(
		ctx,
		proc,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			fmt.Println("stdout:", line)
		}),
	); err != nil {
		return err
	}

	// actually, this should not print if reboot worked
	log.Logger.Infow("successfully rebooted", "command", cmd)
	return nil
}
