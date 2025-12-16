package scan

import (
	"context"
	"fmt"
	"os"
	"runtime"

	apiv1 "github.com/leptonai/gpud/api/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/components"
	nvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/all"
	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func printSummary(result components.CheckResult) {
	header := cmdcommon.CheckMark
	if result.HealthStateType() != apiv1.HealthStateTypeHealthy {
		header = cmdcommon.WarningSign
	}
	fmt.Printf("%s %s\n", header, result.Summary())
	fmt.Println(result.String())
	println()
}

// Runs the scan operations.
func Scan(ctx context.Context, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", cmdcommon.InProgress, runtime.GOOS)

	var nvmlInstance nvidianvml.Instance
	var err error
	if op.failureInjector != nil && (len(op.failureInjector.GPUUUIDsWithGPULost) > 0 ||
		len(op.failureInjector.GPUUUIDsWithGPURequiresReset) > 0 ||
		len(op.failureInjector.GPUUUIDsWithFabricStateHealthSummaryUnhealthy) > 0 ||
		op.failureInjector.GPUProductNameOverride != "") {
		// If failure injector is configured for NVML-level errors or product name override, use it
		nvmlInstance, err = nvidianvml.NewWithFailureInjector(&nvidianvml.FailureInjectorConfig{
			GPUUUIDsWithGPULost:                           op.failureInjector.GPUUUIDsWithGPULost,
			GPUUUIDsWithGPURequiresReset:                  op.failureInjector.GPUUUIDsWithGPURequiresReset,
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: op.failureInjector.GPUUUIDsWithFabricStateHealthSummaryUnhealthy,
			GPUProductNameOverride:                        op.failureInjector.GPUProductNameOverride,
		})
	} else {
		nvmlInstance, err = nvidianvml.New()
	}
	if err != nil {
		return err
	}

	mi, err := pkgmachineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s machine info\n", cmdcommon.CheckMark)
	mi.RenderTable(os.Stdout)

	if mi.GPUInfo != nil && mi.GPUInfo.Product != "" {
		threshold, err := nvidiainfiniband.SupportsInfinibandPortRate(mi.GPUInfo.Product)
		if err == nil {
			log.Logger.Infow("setting default expected port states", "product", mi.GPUInfo.Product, "at_least_ports", threshold.AtLeastPorts, "at_least_rate", threshold.AtLeastRate)
			nvidiainfiniband.SetDefaultExpectedPortStates(threshold)
		}
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,

		MachineID: mi.MachineID,

		NVMLInstance: nvmlInstance,
		NVIDIAToolOverwrites: nvidiacommon.ToolOverwrites{
			InfinibandClassRootDir: op.infinibandClassRootDir,
		},

		EventStore:       nil,
		RebootEventStore: nil,

		MountPoints:  []string{"/"},
		MountTargets: []string{"/var/lib/kubelet"},

		FailureInjector: op.failureInjector,
	}

	for _, c := range all.All() {
		c, err := c.InitFunc(gpudInstance)
		if err != nil {
			return err
		}
		if !c.IsSupported() {
			continue
		}
		printSummary(c.Check())
	}

	fmt.Printf("\n\n%s scan complete\n\n", cmdcommon.CheckMark)
	return nil
}
