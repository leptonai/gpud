package fabric

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// SuggestedActions preserves device-loss/reset guidance while avoiding an
// unsupported hardware diagnosis from registration progress or missing telemetry.
func SuggestedActions(err error) *apiv1.SuggestedActions {
	action := apiv1.RepairActionTypeIgnoreNoActionRequired
	if errors.Is(err, nvmlerrors.ErrGPULost) || errors.Is(err, nvmlerrors.ErrGPURequiresReset) {
		action = apiv1.RepairActionTypeRebootSystem
	}
	return &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{action}}
}

// Report preserves readiness, faults, and unavailable telemetry separately.
type Report struct {
	Entries []device.FabricStateEntry
	Healthy bool
	Pending bool
	Failed  bool
	Reason  string
	Err     error
}

func Collect(devices map[string]device.Device) Report {
	return CollectWith(devices, func(dev device.Device) (device.FabricState, error) { return dev.GetFabricState() })
}

// CollectWith lets callers retain their existing device-query injection point.
func CollectWith(devices map[string]device.Device, get func(device.Device) (device.FabricState, error)) Report {
	r := Report{Healthy: len(devices) > 0}
	if len(devices) == 0 {
		r.Failed = true
		r.Err = errors.New("no GPUs available for fabric registration check")
		r.Reason = r.Err.Error()
		return r
	}
	uuids := make([]string, 0, len(devices))
	for uuid := range devices {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)
	var reasons []string
	for _, uuid := range uuids {
		state, err := get(devices[uuid])
		if err != nil {
			err = fmt.Errorf("fabric state query failed for GPU %s: %w", uuid, err)
			r.Err = errors.Join(r.Err, err)
			r.Failed = true
			reasons = append(reasons, err.Error())
			continue
		}
		r.Entries = append(r.Entries, state.ToEntry(uuid))
		issues := state.GetIssues()
		if state.RegistrationPending() {
			r.Pending = true
			r.Failed = r.Failed || len(state.GetHealthIssues()) > 0
		} else if len(issues) > 0 {
			r.Failed = true
		}
		if len(issues) > 0 {
			reasons = append(reasons, fmt.Sprintf("GPU %s: %s", uuid, strings.Join(issues, ", ")))
		}
	}
	r.Healthy = !r.Pending && !r.Failed
	r.Reason = strings.Join(reasons, "; ")
	return r
}
