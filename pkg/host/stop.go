package host

import (
	"context"
	"fmt"
	stdos "os"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Stop stops the gpud service
func Stop(ctx context.Context, opts ...OpOption) error {
	options := &Op{}
	if err := options.applyOpts(opts); err != nil {
		return err
	}

	asRoot := stdos.Geteuid() == 0 // running as root
	if !asRoot {
		return ErrNotRoot
	}

	cmd := "sudo systemctl stop gpud"

	if options.delaySeconds == 0 {
		log.Logger.Infow("stopping immediately", "command", cmd)
		return runStop(ctx, cmd)
	}

	go func() {
		select {
		case <-time.After(time.Duration(options.delaySeconds) * time.Second):
			log.Logger.Infow("delay expired, stopping now", "command", cmd)
		case <-ctx.Done():
			log.Logger.Warnw("context done, aborting stop", "command", cmd)
			return
		}

		rerr := runStop(ctx, cmd)

		// actually, this should not print if stop worked
		log.Logger.Warnw("successfully stopped", "command", cmd, "error", rerr)
	}()

	log.Logger.Infow(
		"triggering stop after delay",
		"delaySeconds", options.delaySeconds,
		"command", cmd,
	)
	return nil
}

func runStop(ctx context.Context, cmd string) error {
	proc, err := process.New(
		process.WithCommand(cmd),
		process.WithRunAsBashScript(),
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

	// actually, this should not print if stop worked
	log.Logger.Infow("successfully stopped", "command", cmd)
	return nil
}
