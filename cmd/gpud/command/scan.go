package command

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components/diagnose"
	"github.com/leptonai/gpud/log"

	"github.com/urfave/cli"
	"go.uber.org/zap"
)

func cmdScan(cliContext *cli.Context) error {
	var zapLvl zap.AtomicLevel = zap.NewAtomicLevel() // info level by default
	if logLevel != "" && logLevel != "info" {
		lCfg := log.DefaultLoggerConfig()
		var err error
		zapLvl, err = zap.ParseAtomicLevel(logLevel)
		if err != nil {
			return err
		}
		lCfg.Level = zapLvl
		log.Logger = log.CreateLogger(lCfg)
	}

	diagnoseOpts := []diagnose.OpOption{
		diagnose.WithLines(tailLines),
		diagnose.WithPollGPMEvents(pollGPMEvents),
		diagnose.WithNetcheck(netcheck),
		diagnose.WithDiskcheck(diskcheck),
		diagnose.WithDmesgCheck(dmesgCheck),
		diagnose.WithNvidiaSMICommand(nvidiaSMICommand),
		diagnose.WithNvidiaSMIQueryCommand(nvidiaSMIQueryCommand),
		diagnose.WithIbstatCommand(ibstatCommand),
		diagnose.WithCheckIb(checkIb),
	}
	if zapLvl.Level() <= zap.DebugLevel { // e.g., info, warn, error
		diagnoseOpts = append(diagnoseOpts, diagnose.WithDebug(true))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := diagnose.Scan(ctx, diagnoseOpts...)
	if err != nil {
		return err
	}

	return nil
}
