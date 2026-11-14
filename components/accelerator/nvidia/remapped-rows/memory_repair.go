package remappedrows

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// MemoryRepairStatus is the NVML memory-repair state beyond row remapping.
// Rubin restores memory availability with bank remapping and channel repair,
// which NVIDIA reports as inactive row remappings; row counts alone therefore
// under-report pending repairs on Rubin.
// ref. https://docs.nvidia.com/deploy/nvidia-smi/index.html (Bank Remapper, Repair Status)
type MemoryRepairStatus struct {
	// UUID is the GPU UUID.
	UUID string `json:"uuid"`
	// BusID is the GPU bus ID from the nvml API.
	BusID string `json:"bus_id"`

	// BankRemapperSupported is false when the driver or GPU does not implement
	// nvmlDeviceGetBankRemapperStatus_v1 (nvml.h: Rubin or newer).
	BankRemapperSupported bool `json:"bank_remapper_supported"`
	// ActiveBankRemappings is the number of active bank remappings.
	ActiveBankRemappings uint32 `json:"active_bank_remappings"`
	// InactiveBankRemappings is the number of inactive bank remappings.
	InactiveBankRemappings uint32 `json:"inactive_bank_remappings"`
	// BankRemappingPending is nvml.h bPending: "there exists any pending bank remapping".
	BankRemappingPending bool `json:"bank_remapping_pending"`
	// BankRemapperMaxSpareGroups is the number of groups with all spares left.
	BankRemapperMaxSpareGroups uint32 `json:"bank_remapper_max_spare_groups"`
	// BankRemapperNoSpareGroups is the number of groups with no spare left.
	BankRemapperNoSpareGroups uint32 `json:"bank_remapper_no_spare_groups"`

	// RepairStatusSupported is false when nvmlDeviceGetRepairStatus is unavailable.
	RepairStatusSupported bool `json:"repair_status_supported"`
	// ChannelRepairPending is true when a memory channel repair is pending.
	ChannelRepairPending bool `json:"channel_repair_pending"`
	// TPCRepairPending is true when a TPC repair is pending.
	TPCRepairPending bool `json:"tpc_repair_pending"`

	// UnrepairableMemorySupported is false when
	// nvmlDeviceGetUnrepairableMemoryFlag_v1 is unavailable.
	UnrepairableMemorySupported bool `json:"unrepairable_memory_supported"`
	// UnrepairableMemory is true when NVML reports memory it can no longer repair.
	UnrepairableMemory bool `json:"unrepairable_memory"`
}

// RepairPending reports whether a reset is needed to apply a bank remap or a
// channel/TPC repair. nvidia-smi: "If one of the repair flags is pending, check
// the GPU Recovery action".
func (s MemoryRepairStatus) RepairPending() bool {
	return s.BankRemappingPending || s.ChannelRepairPending || s.TPCRepairPending
}

// GetMemoryRepairStatus queries the bank-remapper, repair and unrepairable-memory
// states. Each query is independent, so one failure does not hide the others;
// the returned status holds whatever was read alongside the joined error.
func GetMemoryRepairStatus(uuid string, dev device.Device) (MemoryRepairStatus, error) {
	st := MemoryRepairStatus{UUID: uuid, BusID: dev.PCIBusID()}
	var errs []error

	bank, ret := dev.GetBankRemapperStatus_v1()
	supported, err := memoryRepairQueryResult("bank remapper status", ret)
	if isDeviceGoneError(err) {
		return st, err
	}
	errs = append(errs, err)
	if supported {
		st.BankRemapperSupported = true
		st.ActiveBankRemappings = bank.ActiveRemappings
		st.InactiveBankRemappings = bank.InactiveRemappings
		st.BankRemappingPending = bank.BPending != 0
		st.BankRemapperMaxSpareGroups = bank.Histogram.MaxSpareGroupCount
		st.BankRemapperNoSpareGroups = bank.Histogram.NoSpareGroupCount
	}

	repair, ret := dev.GetRepairStatus()
	supported, err = memoryRepairQueryResult("repair status", ret)
	if isDeviceGoneError(err) {
		return st, err
	}
	errs = append(errs, err)
	if supported {
		st.RepairStatusSupported = true
		st.ChannelRepairPending = repair.BChannelRepairPending != 0
		st.TPCRepairPending = repair.BTpcRepairPending != 0
	}

	unrepairable, ret := dev.GetUnrepairableMemoryFlag_v1()
	supported, err = memoryRepairQueryResult("unrepairable memory flag", ret)
	if isDeviceGoneError(err) {
		return st, err
	}
	errs = append(errs, err)
	if supported {
		st.UnrepairableMemorySupported = true
		st.UnrepairableMemory = unrepairable.BUnrepairableMemory != 0
	}

	return st, errors.Join(errs...)
}

// memoryRepairQueryResult maps an NVML return to (supported, error).
func memoryRepairQueryResult(query string, ret nvml.Return) (bool, error) {
	switch {
	case ret == nvml.SUCCESS:
		return true, nil
	// NOT_SUPPORTED: the GPU lacks the feature. FUNCTION_NOT_FOUND: the
	// driver predates the symbol (see lib.devDevInterface guards). Neither is a
	// GPU fault, so report the state as unsupported instead of failing.
	case nvmlerrors.IsNotSupportError(ret), ret == nvml.ERROR_FUNCTION_NOT_FOUND:
		return false, nil
	// Same sentinels as the row-remapping query so callers handle a lost or
	// reset-required GPU consistently.
	case nvmlerrors.IsGPULostError(ret):
		return false, nvmlerrors.ErrGPULost
	case nvmlerrors.IsGPURequiresReset(ret):
		return false, nvmlerrors.ErrGPURequiresReset
	default:
		return false, fmt.Errorf("failed to get device %s: %v", query, nvml.ErrorString(ret))
	}
}

// isDeviceGoneError stops further queries: every later call on a lost or
// reset-required GPU fails the same way and would only repeat the error.
func isDeviceGoneError(err error) bool {
	return errors.Is(err, nvmlerrors.ErrGPULost) || errors.Is(err, nvmlerrors.ErrGPURequiresReset)
}
