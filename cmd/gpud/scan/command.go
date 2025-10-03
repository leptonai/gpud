package scan

import (
	"context"
	"encoding/json"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/leptonai/gpud/cmd/gpud/common"
	gpudcomponents "github.com/leptonai/gpud/components"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsinfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/scan"
)

func CreateCommand() func(*cli.Context) error {
	return func(cliContext *cli.Context) error {
		return cmdScan(
			cliContext.String("log-level"),
			cliContext.Int("gpu-count"),
			cliContext.String("infiniband-expected-port-states"),
			cliContext.String("nvlink-expected-link-states"),
			cliContext.String("nfs-checker-configs"),
			cliContext.String("infiniband-class-root-dir"),
			cliContext.String("gpu-uuids-with-row-remapping-pending"),
			cliContext.String("gpu-uuids-with-row-remapping-failed"),
			cliContext.String("gpu-uuids-with-hw-slowdown"),
			cliContext.String("gpu-uuids-with-hw-slowdown-thermal"),
			cliContext.String("gpu-uuids-with-hw-slowdown-power-brake"),
			cliContext.Int("xid-reboot-threshold"),
			cliContext.IsSet("xid-reboot-threshold"),
		)
	}
}

func cmdScan(
	logLevel string,
	gpuCount int,
	infinibandExpectedPortStates string,
	nvlinkExpectedLinkStates string,
	nfsCheckerConfigs string,
	ibClassRootDir string,
	gpuUUIDsWithRowRemappingPendingRaw string,
	gpuUUIDsWithRowRemappingFailedRaw string,
	gpuUUIDsWithHWSlowdownRaw string,
	gpuUUIDsWithHWSlowdownThermalRaw string,
	gpuUUIDsWithHWSlowdownPowerBrakeRaw string,
	xidRebootThreshold int,
	xidRebootThresholdIsSet bool,
) error {
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

		log.Logger.Infow("set gpu count", "gpuCount", gpuCount)
	}

	if len(infinibandExpectedPortStates) > 0 {
		var expectedPortStates infiniband.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		componentsinfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
	}

	if len(nvlinkExpectedLinkStates) > 0 {
		var expectedLinkStates componentsnvlink.ExpectedLinkStates
		if err := json.Unmarshal([]byte(nvlinkExpectedLinkStates), &expectedLinkStates); err != nil {
			return err
		}
		componentsnvlink.SetDefaultExpectedLinkStates(expectedLinkStates)

		log.Logger.Infow("set nvlink expected link states", "nvlinkExpectedLinkStates", nvlinkExpectedLinkStates)
	}

	if len(nfsCheckerConfigs) > 0 {
		groupConfigs := make(pkgnfschecker.Configs, 0)
		if err := json.Unmarshal([]byte(nfsCheckerConfigs), &groupConfigs); err != nil {
			return err
		}
		componentsnfs.SetDefaultConfigs(groupConfigs)

		log.Logger.Infow("set nfs checker group configs", "groupConfigs", groupConfigs)
	}

	if xidRebootThresholdIsSet {
		if xidRebootThreshold > 0 {
			componentsxid.SetDefaultRebootThreshold(componentsxid.RebootThreshold{
				Threshold: xidRebootThreshold,
			})
			log.Logger.Infow("set xid reboot threshold", "xidRebootThreshold", xidRebootThreshold)
		} else {
			log.Logger.Warnw("ignoring xid reboot threshold override, value must be positive", "xidRebootThreshold", xidRebootThreshold)
		}
	}

	gpuUUIDsWithRowRemappingPending := common.ParseGPUUUIDs(gpuUUIDsWithRowRemappingPendingRaw)
	gpuUUIDsWithRowRemappingFailed := common.ParseGPUUUIDs(gpuUUIDsWithRowRemappingFailedRaw)
	gpuUUIDsWithHWSlowdown := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownRaw)
	gpuUUIDsWithHWSlowdownThermal := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownThermalRaw)
	gpuUUIDsWithHWSlowdownPowerBrake := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownPowerBrakeRaw)

	opts := []scan.OpOption{
		scan.WithInfinibandClassRootDir(ibClassRootDir),
		scan.WithFailureInjector(&gpudcomponents.FailureInjector{
			GPUUUIDsWithRowRemappingPending:  gpuUUIDsWithRowRemappingPending,
			GPUUUIDsWithRowRemappingFailed:   gpuUUIDsWithRowRemappingFailed,
			GPUUUIDsWithHWSlowdown:           gpuUUIDsWithHWSlowdown,
			GPUUUIDsWithHWSlowdownThermal:    gpuUUIDsWithHWSlowdownThermal,
			GPUUUIDsWithHWSlowdownPowerBrake: gpuUUIDsWithHWSlowdownPowerBrake,
		}),
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
