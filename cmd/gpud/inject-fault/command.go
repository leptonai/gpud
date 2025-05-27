// Package injectfault provides a command to inject faults into the system.
package injectfault

import (
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

	log.Logger.Debugw("starting inject-fault command")

	kernelLogLevel := cliContext.String("kernel-log-level")
	kernelMsg := cliContext.String("kernel-message")

	wr := pkgkmsgwriter.NewWriter(pkgkmsgwriter.DefaultDevKmsg)
	if err := wr.Write(&pkgkmsgwriter.KernelMessage{
		Priority: pkgkmsgwriter.KernelMessagePriority(kernelLogLevel),
		Message:  kernelMsg,
	}); err != nil {
		return err
	}

	return nil
}
