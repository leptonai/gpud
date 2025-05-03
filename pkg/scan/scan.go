package scan

import (
	"context"
	"fmt"
	"os"
	"runtime"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidiainfiniband "github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

	componentsacceleratornvidiabadenvs "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs"
	componentsacceleratornvidiaclockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	componentsacceleratornvidiaecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	componentsacceleratornvidiafabricmanager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	componentsacceleratornvidiagpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	componentsacceleratornvidiagspfirmwaremode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	componentsacceleratornvidiahwslowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	componentsacceleratornvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsacceleratornvidiainfo "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	componentsacceleratornvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	componentsacceleratornvidianccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	componentsacceleratornvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentsacceleratornvidiapeermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	componentsacceleratornvidiapersistencemode "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode"
	componentsacceleratornvidiapower "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	componentsacceleratornvidiaprocesses "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	componentsacceleratornvidiaremappedrows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	componentsacceleratornvidiasxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	componentsacceleratornvidiatemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsacceleratornvidiautilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	componentsacceleratornvidiaxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentscontainerdpod "github.com/leptonai/gpud/components/containerd/pod"
	componentscpu "github.com/leptonai/gpud/components/cpu"
	componentsdisk "github.com/leptonai/gpud/components/disk"
	componentsdockercontainer "github.com/leptonai/gpud/components/docker/container"
	componentsfd "github.com/leptonai/gpud/components/fd"
	componentsfuse "github.com/leptonai/gpud/components/fuse"
	componentsinfo "github.com/leptonai/gpud/components/info"
	componentskernelmodule "github.com/leptonai/gpud/components/kernel-module"
	componentskubeletpod "github.com/leptonai/gpud/components/kubelet/pod"
	componentslibrary "github.com/leptonai/gpud/components/library"
	componentsmemory "github.com/leptonai/gpud/components/memory"
	componentsnetworklatency "github.com/leptonai/gpud/components/network/latency"
	componentsos "github.com/leptonai/gpud/components/os"
	componentspci "github.com/leptonai/gpud/components/pci"
	componentstailscale "github.com/leptonai/gpud/components/tailscale"
)

var componentInits = []components.InitFunc{
	componentscpu.New,
	componentscontainerdpod.New,
	componentsdisk.New,
	componentsdockercontainer.New,
	componentsfd.New,
	componentsfuse.New,
	componentsinfo.New,
	componentskernelmodule.New,
	componentskubeletpod.New,
	componentslibrary.New,
	componentsmemory.New,
	componentsnetworklatency.New,
	componentsos.New,
	componentspci.New,
	componentstailscale.New,
	componentsacceleratornvidiabadenvs.New,
	componentsacceleratornvidiaclockspeed.New,
	componentsacceleratornvidiaecc.New,
	componentsacceleratornvidiafabricmanager.New,
	componentsacceleratornvidiagpm.New,
	componentsacceleratornvidiagspfirmwaremode.New,
	componentsacceleratornvidiahwslowdown.New,
	componentsacceleratornvidiainfiniband.New,
	componentsacceleratornvidiainfo.New,
	componentsacceleratornvidiamemory.New,
	componentsacceleratornvidianccl.New,
	componentsacceleratornvidianvlink.New,
	componentsacceleratornvidiapeermem.New,
	componentsacceleratornvidiapersistencemode.New,
	componentsacceleratornvidiapower.New,
	componentsacceleratornvidiaprocesses.New,
	componentsacceleratornvidiaremappedrows.New,
	componentsacceleratornvidiasxid.New,
	componentsacceleratornvidiatemperature.New,
	componentsacceleratornvidiautilization.New,
	componentsacceleratornvidiaxid.New,
}

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

	for _, initFunc := range componentInits {
		c, err := initFunc(gpudInstance)
		if err != nil {
			return err
		}
		printSummary(c.Check())
	}

	fmt.Printf("\n\n%s scan complete\n\n", checkMark)
	return nil
}
