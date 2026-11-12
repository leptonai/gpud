package fabric

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// SuggestedActions preserves device-loss/reset guidance while avoiding an
// unsupported host recovery or hardware diagnosis from fabric unavailability.
func SuggestedActions(err error, health apiv1.HealthStateType) *apiv1.SuggestedActions {
	action := apiv1.RepairActionTypeIgnoreNoActionRequired
	if errors.Is(err, nvmlerrors.ErrGPULost) || errors.Is(err, nvmlerrors.ErrGPURequiresReset) {
		action = apiv1.RepairActionTypeRebootSystem
	}
	actions := &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{action}}
	if health == apiv1.HealthStateTypeUnhealthy && action == apiv1.RepairActionTypeIgnoreNoActionRequired {
		actions.Description = "Inspect fabric management, switch connectivity, and driver compatibility; fabric unavailability alone does not establish a host reboot or hardware repair requirement"
	}
	return actions
}

// Report preserves readiness, faults, and unavailable telemetry separately.
type Report struct {
	Entries        []device.FabricStateEntry
	AllUnsupported bool
	Healthy        bool
	Pending        bool
	Failed         bool
	Reason         string
	Err            error
}

func Collect(devices map[string]device.Device) Report {
	return CollectWith(devices, func(dev device.Device) (device.FabricState, error) { return dev.GetFabricState() })
}

// CollectWith lets callers retain their existing device-query injection point.
func CollectWith(devices map[string]device.Device, get func(device.Device) (device.FabricState, error)) Report {
	r := Report{Healthy: len(devices) > 0, AllUnsupported: len(devices) > 0}
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
			r.AllUnsupported = r.AllUnsupported && errors.Is(err, nvml.ERROR_NOT_SUPPORTED)
			err = fmt.Errorf("fabric state query failed for GPU %s: %w", uuid, err)
			r.Err = errors.Join(r.Err, err)
			r.Failed = true
			reasons = append(reasons, err.Error())
			continue
		}
		unsupported := state.State == nvml.GPU_FABRIC_STATE_NOT_SUPPORTED &&
			(state.Status == nvml.SUCCESS || state.Status == nvml.ERROR_NOT_SUPPORTED || state.Status == nvml.ERROR_UNKNOWN) &&
			state.HealthSummary == nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED && state.HealthMask == 0
		r.AllUnsupported = r.AllUnsupported && unsupported
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

// UnsupportedWithoutFabric distinguishes an absent API from a failed fabric.
// An explicit NVL72 topology or a detected switch still requires telemetry.
func (r Report) UnsupportedWithoutFabric(product string, switchDetected bool) bool {
	return r.AllUnsupported && !switchDetected && !strings.Contains(strings.ToLower(product), "nvl72")
}
