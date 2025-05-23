// Package all contains all the components.
package all

import (
	"github.com/leptonai/gpud/components"

	componentsacceleratornvidiabadenvs "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs"
	componentsacceleratornvidiaclockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	componentsacceleratornvidiaecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	componentsacceleratornvidiafabricmanager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	componentsacceleratornvidiagpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	componentsacceleratornvidiagspfirmwaremode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	componentsacceleratornvidiahwslowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	componentsacceleratornvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
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
	componentsfuse "github.com/leptonai/gpud/components/fuse"
	componentskernelmodule "github.com/leptonai/gpud/components/kernel-module"
	componentskubeletpod "github.com/leptonai/gpud/components/kubelet/pod"
	componentslibrary "github.com/leptonai/gpud/components/library"
	componentsmemory "github.com/leptonai/gpud/components/memory"
	componentsnetworklatency "github.com/leptonai/gpud/components/network/latency"
	componentsos "github.com/leptonai/gpud/components/os"
	componentspci "github.com/leptonai/gpud/components/pci"
	componentstailscale "github.com/leptonai/gpud/components/tailscale"
)

func InitFuncs() []components.InitFunc {
	return componentInits
}

var componentInits = []components.InitFunc{
	componentscpu.New,
	componentscontainerdpod.New,
	componentsdisk.New,
	componentsdockercontainer.New,
	componentsfuse.New,
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
