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
	componentscontainerd "github.com/leptonai/gpud/components/containerd"
	componentscpu "github.com/leptonai/gpud/components/cpu"
	componentsdisk "github.com/leptonai/gpud/components/disk"
	componentsdocker "github.com/leptonai/gpud/components/docker"
	componentsfuse "github.com/leptonai/gpud/components/fuse"
	componentskernelmodule "github.com/leptonai/gpud/components/kernel-module"
	componentskubelet "github.com/leptonai/gpud/components/kubelet"
	componentslibrary "github.com/leptonai/gpud/components/library"
	componentsmemory "github.com/leptonai/gpud/components/memory"
	componentsnetworklatency "github.com/leptonai/gpud/components/network/latency"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	componentsos "github.com/leptonai/gpud/components/os"
	componentspci "github.com/leptonai/gpud/components/pci"
	componentstailscale "github.com/leptonai/gpud/components/tailscale"
)

type Component struct {
	Name     string
	InitFunc components.InitFunc
}

func All() []Component {
	return componentInits
}

var componentInits = []Component{
	{Name: componentsacceleratornvidiabadenvs.Name, InitFunc: componentsacceleratornvidiabadenvs.New},
	{Name: componentsacceleratornvidiaclockspeed.Name, InitFunc: componentsacceleratornvidiaclockspeed.New},
	{Name: componentsacceleratornvidiaecc.Name, InitFunc: componentsacceleratornvidiaecc.New},
	{Name: componentsacceleratornvidiafabricmanager.Name, InitFunc: componentsacceleratornvidiafabricmanager.New},
	{Name: componentsacceleratornvidiagpm.Name, InitFunc: componentsacceleratornvidiagpm.New},
	{Name: componentsacceleratornvidiagspfirmwaremode.Name, InitFunc: componentsacceleratornvidiagspfirmwaremode.New},
	{Name: componentsacceleratornvidiahwslowdown.Name, InitFunc: componentsacceleratornvidiahwslowdown.New},
	{Name: componentsacceleratornvidiainfiniband.Name, InitFunc: componentsacceleratornvidiainfiniband.New},
	{Name: componentsacceleratornvidiamemory.Name, InitFunc: componentsacceleratornvidiamemory.New},
	{Name: componentsacceleratornvidianccl.Name, InitFunc: componentsacceleratornvidianccl.New},
	{Name: componentsacceleratornvidianvlink.Name, InitFunc: componentsacceleratornvidianvlink.New},
	{Name: componentsacceleratornvidiapeermem.Name, InitFunc: componentsacceleratornvidiapeermem.New},
	{Name: componentsacceleratornvidiapersistencemode.Name, InitFunc: componentsacceleratornvidiapersistencemode.New},
	{Name: componentsacceleratornvidiapower.Name, InitFunc: componentsacceleratornvidiapower.New},
	{Name: componentsacceleratornvidiaprocesses.Name, InitFunc: componentsacceleratornvidiaprocesses.New},
	{Name: componentsacceleratornvidiaremappedrows.Name, InitFunc: componentsacceleratornvidiaremappedrows.New},
	{Name: componentsacceleratornvidiasxid.Name, InitFunc: componentsacceleratornvidiasxid.New},
	{Name: componentsacceleratornvidiatemperature.Name, InitFunc: componentsacceleratornvidiatemperature.New},
	{Name: componentsacceleratornvidiautilization.Name, InitFunc: componentsacceleratornvidiautilization.New},
	{Name: componentsacceleratornvidiaxid.Name, InitFunc: componentsacceleratornvidiaxid.New},
	{Name: componentscontainerd.Name, InitFunc: componentscontainerd.New},
	{Name: componentscpu.Name, InitFunc: componentscpu.New},
	{Name: componentsdisk.Name, InitFunc: componentsdisk.New},
	{Name: componentsdocker.Name, InitFunc: componentsdocker.New},
	{Name: componentsfuse.Name, InitFunc: componentsfuse.New},
	{Name: componentskernelmodule.Name, InitFunc: componentskernelmodule.New},
	{Name: componentskubelet.Name, InitFunc: componentskubelet.New},
	{Name: componentslibrary.Name, InitFunc: componentslibrary.New},
	{Name: componentsmemory.Name, InitFunc: componentsmemory.New},
	{Name: componentsnetworklatency.Name, InitFunc: componentsnetworklatency.New},
	{Name: componentsnfs.Name, InitFunc: componentsnfs.New},
	{Name: componentsos.Name, InitFunc: componentsos.New},
	{Name: componentspci.Name, InitFunc: componentspci.New},
	{Name: componentstailscale.Name, InitFunc: componentstailscale.New},
}
