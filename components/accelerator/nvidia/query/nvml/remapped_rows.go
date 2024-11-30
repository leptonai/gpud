package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/log"
)

// RemappedRows represents the row remapping data.
// The row remapping feature is used to prevent known degraded memory locations from being used.
// But may require a GPU reset to actually remap the rows.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#row-remapping
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
type RemappedRows struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// The number of rows remapped due to correctable errors.
	RemappedDueToCorrectableErrors int `json:"remapped_due_to_correctable_errors"`

	// The number of rows remapped due to uncorrectable errors.
	RemappedDueToUncorrectableErrors int `json:"remapped_due_to_uncorrectable_errors"`

	// Indicates whether or not remappings are pending.
	// If true, GPU requires a reset to actually remap the row.
	//
	// A pending remapping won't affect future work on the GPU
	// since error-containment and dynamic page blacklisting will take care of that.
	RemappingPending bool `json:"remapping_pending"`

	// Set to true when a remapping has failed in the past.
	// A pending remapping won't affect future work on the GPU
	// since error-containment and dynamic page blacklisting will take care of that.
	RemappingFailed bool `json:"remapping_failed"`
}

func GetRemappedRows(uuid string, dev device.Device) (RemappedRows, error) {
	remRws := RemappedRows{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
	corrRows, uncRows, isPending, failureOccurred, ret := dev.GetRemappedRows()
	if ret != nvml.SUCCESS {
		return RemappedRows{}, fmt.Errorf("failed to get device remapped rows: %v", nvml.ErrorString(ret))
	}
	remRws.RemappedDueToCorrectableErrors = corrRows
	remRws.RemappedDueToUncorrectableErrors = uncRows
	remRws.RemappingPending = isPending
	remRws.RemappingFailed = failureOccurred

	return remRws, nil
}

// Returns true if a GPU requires a reset to remap the rows.
func (r RemappedRows) RequiresReset() bool {
	// "isPending indicates whether or not there are pending remappings. A reset will be required to actually remap the row."
	return r.RemappingPending
}

// Returns true if a GPU qualifies for RMA.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#rma-policy-thresholds-for-row-remapping
func (r RemappedRows) QualifiesForRMA() bool {
	// "Regarding row-remapping failures, the RMA criteria is met when the row-remapping failure flag is set and validated by the field diagnostic."
	// "Any of the following events will trigger a row-remapping failure flag:"
	// "remapping attempt for an uncorrectable memory error on a bank that already has eight uncorrectable error rows remapped."
	// "r.RemappedDueToUncorrectableErrors >= 8" was dropped since it is also possible that:
	// "A remapping attempt for an uncorrectable memory error on a row that was already remapped and can occur with less than eight total remaps to the same bank."
	if r.RemappingFailed && r.RemappedDueToUncorrectableErrors < 8 {
		log.Logger.Debugw("uncorrectable error count <8 but still qualifies for RMA since remapping failed", "uncorrectableErrors", r.RemappedDueToUncorrectableErrors)
	}

	return r.RemappingFailed
}
