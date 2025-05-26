package customplugins

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/urfave/cli"

	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
	"github.com/leptonai/gpud/pkg/log"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting write-kmsg command")

	if err := pkgkmsgwriter.CheckPermissions(); err != nil {
		return err
	}

	msg := cliContext.Args().First()
	if msg == "" {
		return errors.New("message is required")
	}

	workDir, err := os.MkdirTemp(os.TempDir(), "gpud-cmd-write-kmsg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	module := pkgkmsgwriter.NewGPUdKmsgWriterModule(ctx, workDir)
	if err := module.BuildInstall(ctx); err != nil {
		return err
	}

	kernelLogLevel := cliContext.String("kernel-log-level")
	if err := module.InjectKernelMessage(ctx, &pkgkmsgwriter.KernelMessage{
		Priority: pkgkmsgwriter.KernelMessagePriority(kernelLogLevel),
		Message:  msg,
	}); err != nil {
		return err
	}

	if err := module.Uninstall(ctx); err != nil {
		return err
	}

	return nil
}
