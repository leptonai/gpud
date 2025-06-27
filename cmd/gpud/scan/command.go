package scan

import (
	"context"
	"encoding/json"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/scan"
)

func CreateCommand() func(*cli.Context) error {
	return func(cliContext *cli.Context) error {
		return cmdScan(
			cliContext.String("log-level"),
			cliContext.Int("gpu-count"),
			cliContext.String("ibstat-command"),
			cliContext.String("nfs-checker-configs"),
		)
	}
}

func cmdScan(logLevel string, gpuCount int, ibstatCommand string, nfsCheckerConfigs string) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting scan command")

	if gpuCount > 0 {
		componentsnvidiagpucounts.SetDefaultExpectedGPUCounts(componentsnvidiagpucounts.ExpectedGPUCounts{
			Count: gpuCount,
		})
	}

	if len(nfsCheckerConfigs) > 0 {
		groupConfigs := make(pkgnfschecker.Configs, 0)
		if err := json.Unmarshal([]byte(nfsCheckerConfigs), &groupConfigs); err != nil {
			return err
		}
		componentsnfs.SetDefaultConfigs(groupConfigs)
		log.Logger.Debugw("set nfs checker group configs", "groupConfigs", groupConfigs)
	}

	opts := []scan.OpOption{
		scan.WithIbstatCommand(ibstatCommand),
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
