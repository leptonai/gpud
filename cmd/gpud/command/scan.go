package command

import (
	"context"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/leptonai/gpud/pkg/diagnose"
	"github.com/leptonai/gpud/pkg/log"
)

func cmdScan(cliContext *cli.Context) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	diagnoseOpts := []diagnose.OpOption{
		diagnose.WithDiskcheck(diskcheck),
		diagnose.WithKMsgCheck(kmsgCheck),
		diagnose.WithIbstatCommand(ibstatCommand),
		diagnose.WithCheckInfiniband(checkInfiniBand),
	}
	if zapLvl.Level() <= zap.DebugLevel { // e.g., info, warn, error
		diagnoseOpts = append(diagnoseOpts, diagnose.WithDebug(true))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = diagnose.Scan(ctx, diagnoseOpts...); err != nil {
		return err
	}

	return nil
}
