package command

import (
	"context"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/scan"
)

func cmdScan(cliContext *cli.Context) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	opts := []scan.OpOption{
		scan.WithNetcheck(netcheck),
		scan.WithDiskcheck(diskcheck),
		scan.WithKMsgCheck(kmsgCheck),
		scan.WithIbstatCommand(ibstatCommand),
		scan.WithCheckInfiniband(checkInfiniBand),
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
