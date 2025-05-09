package scan

import (
	"context"
	"fmt"
	"os"
	"runtime"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentsacceleratornvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/all"
	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidiainfiniband "github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

func printSummary(result components.CheckResult) {
	header := checkMark
	if result.HealthStateType() != apiv1.HealthStateTypeHealthy {
		header = warningSign
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

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", inProgress, runtime.GOOS)

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return err
	}

	mi, err := pkgmachineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s machine info\n", checkMark)
	mi.RenderTable(os.Stdout)

	if mi.GPUInfo != nil && mi.GPUInfo.Product != "" {
		threshold, err := nvidiainfiniband.SupportsInfinibandPortRate(mi.GPUInfo.Product)
		if err == nil {
			log.Logger.Infow("setting default expected port states", "product", mi.GPUInfo.Product, "at_least_ports", threshold.AtLeastPorts, "at_least_rate", threshold.AtLeastRate)
			componentsacceleratornvidiainfiniband.SetDefaultExpectedPortStates(threshold)
		}
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,

		NVMLInstance: nvmlInstance,
		NVIDIAToolOverwrites: nvidiacommon.ToolOverwrites{
			IbstatCommand:   op.ibstatCommand,
			IbstatusCommand: op.ibstatusCommand,
		},

		EventStore:       nil,
		RebootEventStore: nil,

		MountPoints:  []string{"/"},
		MountTargets: []string{"/var/lib/kubelet"},
	}

	for _, initFunc := range all.InitFuncs() {
		c, err := initFunc(gpudInstance)
		if err != nil {
			return err
		}
		if !c.IsSupported() {
			continue
		}
		printSummary(c.Check())
	}

	fmt.Printf("\n\n%s scan complete\n\n", checkMark)
	return nil
}
