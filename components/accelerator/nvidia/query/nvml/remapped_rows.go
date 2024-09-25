package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
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
	CorrectableErrors int `json:"correctable_errors"`

	// The number of rows remapped due to uncorrectable errors.
	UncorrectableErrors int `json:"uncorrectable_errors"`

	// Indicates whether or not remappings are pending.
	// If true, GPU requires a reset to actually remap the row.
	//
	// A pending remapping won't affect future work on the GPU
	// since error-containment and dynamic page blacklisting will take care of that.
	IsPending bool `json:"is_pending"`

	// Set to true when a remapping has failed in the past.
	// A pending remapping won't affect future work on the GPU
	// since error-containment and dynamic page blacklisting will take care of that.
	FailureOccurred bool `json:"failure_occurred"`
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
	remRws.CorrectableErrors = corrRows
	remRws.UncorrectableErrors = uncRows
	remRws.IsPending = isPending
	remRws.FailureOccurred = failureOccurred

	return remRws, nil
}

// Returns true if a GPU qualifies for RMA.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#rma-policy-thresholds-for-row-remapping
func (r RemappedRows) QualifiesForRMA() bool {
	// "remapping attempt for an uncorrectable memory error on a bank that already has eight uncorrectable error rows remapped."
	if r.FailureOccurred && r.UncorrectableErrors >= 8 {
		return true
	}
	return false
}

// Returns true if a GPU requires a reset to remap the rows.
func (r RemappedRows) RequiresGPUReset() bool {
	// "isPending indicates whether or not there are pending remappings. A reset will be required to actually remap the row."
	return r.IsPending
}
