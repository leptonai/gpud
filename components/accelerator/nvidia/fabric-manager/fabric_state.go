package fabricmanager

import (
	"errors"
	"fmt"

	"io"
	"sort"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type fabricStateReport struct {
	Entries        []device.FabricStateEntry
	AllUnsupported bool
	Healthy        bool
	Reason         string
	Err            error
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

	devices := nvmlInstance.Devices()
	if len(devices) == 0 {
		return report
	}

	report.AllUnsupported = true
	uuids := make([]string, 0, len(devices))
	for uuid := range devices {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)

	reasons := make([]string, 0)

	for _, uuid := range uuids {
		dev := devices[uuid]
		info, err := getFabricInfoFn(dev)
		if err != nil {
			report.Err = fmt.Errorf("fabric state query failed for GPU %s: %w", uuid, err)
			report.Healthy = false
			if errors.Is(err, nvml.ERROR_NOT_SUPPORTED) {
				continue
			}
			report.AllUnsupported = false
			return report
		}

		entry := info.ToEntry(uuid)
		issues := info.GetIssues()
		unsupported := info.State == nvml.GPU_FABRIC_STATE_NOT_SUPPORTED &&
			(info.Status == nvml.SUCCESS || info.Status == nvml.ERROR_NOT_SUPPORTED || info.Status == nvml.ERROR_UNKNOWN) &&
			info.HealthSummary == nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED && info.HealthMask == 0
		report.AllUnsupported = report.AllUnsupported && unsupported
		report.Entries = append(report.Entries, entry)
		if len(issues) > 0 {
			report.Healthy = false
			reasons = append(reasons, fmt.Sprintf("GPU %s: %s", uuid, strings.Join(issues, ", ")))
		}
	}

	if len(reasons) > 0 {
		sort.Strings(reasons)
		report.Reason = strings.Join(reasons, "; ")
	}

	return report
}
