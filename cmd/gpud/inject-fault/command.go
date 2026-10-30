// Package injectfault provides a command to inject faults into the system.
package injectfault

import (
	"github.com/urfave/cli"

	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	"github.com/leptonai/gpud/pkg/log"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

	log.Logger.Debugw("starting inject-fault command")

	kernelLogLevel := cliContext.String("kernel-log-level")
	kernelMsg := cliContext.String("kernel-message")

	wr := pkgfaultinjector.NewWriter(pkgfaultinjector.DefaultDevKmsg)
	if err := wr.Write(&pkgfaultinjector.KernelMessage{
		Priority: pkgfaultinjector.KernelMessagePriority(kernelLogLevel),
		Message:  kernelMsg,
	}); err != nil {
		return err
	}

	return nil
}
