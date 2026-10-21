package firmware

import (
	"sort"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// collectGPU reads the VBIOS version of every GPU and names the GPUs gpu1..gpuN
// by PCI bus ID, which matches the nvidia-smi index plus one under the default
// enumeration order.
func collectGPU(nvmlInstance nvidianvml.Instance) []instance {
	if nvmlInstance == nil {
		return nil
	}
	devs := nvmlInstance.Devices()
	if len(devs) == 0 {
		return nil
	}

	// Devices() is a map, so sort for stable names.
	sorted := make([]device.Device, 0, len(devs))
	for _, dev := range devs {
		if dev != nil {
			sorted = append(sorted, dev)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].PCIBusID() != sorted[j].PCIBusID() {
			return sorted[i].PCIBusID() < sorted[j].PCIBusID()
		}
		return sorted[i].UUID() < sorted[j].UUID()
	})

	found := make([]instance, 0, len(sorted))
	for idx, dev := range sorted {
		// Names are assigned before failed reads are skipped, so one bad GPU
		// never renames the others.
		name := "gpu" + strconv.Itoa(idx+1)

		version, ret := dev.GetVbiosVersion()
		if ret != nvml.SUCCESS {
			log.Logger.Debugw("skipping gpu firmware", "gpu", name, "busID", dev.PCIBusID(), "error", nvml.ErrorString(ret))
			continue
		}
		version = strings.TrimSpace(version)
		if version == "" {
			continue
		}
		found = append(found, instance{component: apiv1.FirmwareComponentGPU, version: version, name: name})
	}
	return found
}
