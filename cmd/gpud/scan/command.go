// Package scan implements the "scan" command.
package scan

import (
	"context"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/scan"
)

func CreateCommand() func(*cli.Context) error {
	return func(cliContext *cli.Context) error {
		return cmdScan(
			cliContext.String("log-level"),
			cliContext.String("ibstat-command"),
			cliContext.String("ibstatus-command"),
		)
	}
}

func cmdScan(logLevel string, ibstatCommand string, ibstatusCommand string) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("start scan command")

	opts := []scan.OpOption{
		scan.WithIbstatCommand(ibstatCommand),
		scan.WithIbstatusCommand(ibstatusCommand),
	}
	if zapLvl.Level() <= zap.DebugLevel { // e.g., info, warn, error
		opts = append(opts, scan.WithDebug(true))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = scan.Scan(ctx, opts...); err != nil {
		return err
	}

	return nil
}
