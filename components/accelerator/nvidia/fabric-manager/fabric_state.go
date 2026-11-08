package fabricmanager

import (
	"fmt"
	"io"

	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"

	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type fabricStateReport struct {
	Entries []device.FabricStateEntry
	Healthy bool
	Pending bool
	Failed  bool
	Reason  string
	Err     error
}

// RenderTable renders the fabric state report as a formatted table
func (r fabricStateReport) RenderTable(wr io.Writer) {
	if len(r.Entries) == 0 {
		_, _ = wr.Write([]byte("No fabric state entries\n"))
		return
	}

	for i, entry := range r.Entries {
		if i > 0 {
			_, _ = wr.Write([]byte("\n"))
		}
		entry.RenderTable(wr)
	}

	if !r.Healthy && r.Reason != "" {
		_, _ = fmt.Fprintf(wr, "\nOverall Status: UNHEALTHY - %s\n", r.Reason)
	} else if r.Healthy {
		_, _ = fmt.Fprintf(wr, "\nOverall Status: HEALTHY\n")
	}
}

// fabricStateGetter interface for devices that support GetFabricState
type fabricStateGetter interface {
	GetFabricState() (device.FabricState, error)
}

// getFabricInfo retrieves fabric state information from a device using the Device abstraction.
// This method delegates to Device.GetFabricState() which handles V3/V1 API fallback internally.
func getFabricInfo(dev any) (device.FabricState, error) {
	if dev == nil {
		return device.FabricState{}, fmt.Errorf("nil device handle")
	}

	// Use the Device abstraction's GetFabricState method
	if d, ok := dev.(fabricStateGetter); ok {
		return d.GetFabricState()
	}

	return device.FabricState{}, fmt.Errorf("device does not support GetFabricState()")
}

// getFabricInfoFn is used to allow tests to override NVML querying logic.
// In production it points to getFabricInfo.
var getFabricInfoFn = getFabricInfo

// collectFabricState collects fabric state information from all GPU devices.
// It queries each GPU for fabric state via NVML APIs and returns a comprehensive report.
func collectFabricState(nvmlInstance nvidianvml.Instance) fabricStateReport {
	report := fabricStateReport{Healthy: true}

	if nvmlInstance == nil {
		report.Err = fmt.Errorf("nvml instance is nil")
		report.Healthy = false
		return report
	}

	r := fabric.CollectWith(nvmlInstance.Devices(), func(dev device.Device) (device.FabricState, error) {
		return getFabricInfoFn(dev)
	})
	return fabricStateReport(r)
}
