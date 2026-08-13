package common

import (
	"context"
	"strings"
	"time"

	"github.com/urfave/cli"

	componentsos "github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/pkg/log"
	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
)

// DetectNvidiaGPU reports whether the machine has NVIDIA GPU devices, detected
// via lspci "3D controller" lines. lspci reads PCI configuration space
// directly, so detection works even when the NVIDIA driver/NVML is wedged --
// the exact failure mode D-state process tracking targets (LEP-6029).
func DetectNvidiaGPU(ctx context.Context) (bool, error) {
	devs, err := nvidiapci.ListPCIGPUs(ctx)
	if err != nil {
		return false, err
	}
	return len(devs) > 0, nil
}

// ApplyOSBlockedProcessThresholds resolves and applies the os component's
// D-state blocked-process thresholds (LEP-6029) from CLI flags, auto-enabling
// detection on machines with NVIDIA GPUs:
//
//   - explicit flags win; an explicitly empty --os-blocked-process-name-regexes
//     disables the check even on GPU machines
//   - otherwise, when an NVIDIA GPU is detected, the name regexes default to
//     matching NVIDIA processes (e.g., nvidia-smi) so a released gpud detects
//     D-state issues out of the box
//   - oneOffScan (gpud scan) lowers the auto persistence threshold to 1, since
//     a one-off check cannot observe consecutive failures
//   - without an NVIDIA GPU and without flags, the built-in default (empty
//     regexes) keeps D-state checking disabled
func ApplyOSBlockedProcessThresholds(cliContext *cli.Context, detectGPU func(ctx context.Context) (bool, error), oneOffScan bool) error {
	regexesSet := cliContext.IsSet("os-blocked-process-name-regexes")
	thresholdSet := cliContext.IsSet("os-blocked-process-persistence-threshold")

	thresholds := componentsos.GetDefaultBlockedProcessThresholds()

	if (!regexesSet || !thresholdSet) && detectGPU != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		hasGPU, err := detectGPU(ctx)
		if err != nil {
			log.Logger.Warnw("failed to detect NVIDIA GPUs; leaving os blocked-process thresholds at defaults", "error", err)
		} else if hasGPU {
			if !regexesSet {
				thresholds.NameRegexes = componentsos.DefaultBlockedProcessNameRegexes()
			}
			if !thresholdSet && oneOffScan {
				thresholds.PersistenceThreshold = 1
			}
		}
	}

	if regexesSet {
		thresholds.NameRegexes = strings.Split(cliContext.String("os-blocked-process-name-regexes"), ",")
	}
	if thresholdSet {
		thresholds.PersistenceThreshold = cliContext.Int("os-blocked-process-persistence-threshold")
	}

	if err := componentsos.SetDefaultBlockedProcessThresholds(thresholds); err != nil {
		return err
	}
	// record the startup-resolved thresholds so the session updateConfig
	// fallback can restore them when the control-plane config omits the os
	// component (otherwise unrelated config pushes would silently disable
	// D-state tracking on GPU machines)
	componentsos.SetStartupBlockedProcessThresholds(componentsos.GetDefaultBlockedProcessThresholds())
	log.Logger.Infow("applied os blocked process thresholds",
		"persistenceThreshold", componentsos.GetDefaultBlockedProcessThresholds().PersistenceThreshold,
		"nameRegexes", componentsos.GetDefaultBlockedProcessThresholds().NameRegexes,
		"oneOffScan", oneOffScan,
	)
	return nil
}
