package remappedrows

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return nil
	}

	o := &Output{
		GPUProductName:                    i.GPUProductName(),
		MemoryErrorManagementCapabilities: i.MemoryErrorManagementCapabilities,
	}

	needRebootMsgs := make([]string, 0)
	rmaMsgs := make([]string, 0)

	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.RemappedRowsNVML = append(o.RemappedRowsNVML, device.RemappedRows)

			requiresReset := device.RemappedRows.RequiresReset()
			if requiresReset {
				msg := fmt.Sprintf("GPU %s needs reset (detected pending row remapping)", device.UUID)
				needRebootMsgs = append(needRebootMsgs, msg)
			}

			rma := device.RemappedRows.QualifiesForRMA()
			if rma {
				msg := fmt.Sprintf("GPU %s qualifies for RMA (row remapping failed)", device.UUID)
				rmaMsgs = append(rmaMsgs, msg)
			}
		}
	}

	if len(needRebootMsgs) > 0 {
		if o.SuggestedActions == nil {
			o.SuggestedActions = &common.SuggestedActions{}
		}

		o.SuggestedActions.Descriptions = append(o.SuggestedActions.Descriptions, strings.Join(needRebootMsgs, ", "))
		o.SuggestedActions.RepairActions = append(o.SuggestedActions.RepairActions, common.RepairActionTypeRebootSystem)
	}
	if len(rmaMsgs) > 0 {
		if o.SuggestedActions == nil {
			o.SuggestedActions = &common.SuggestedActions{}
		}

		o.SuggestedActions.Descriptions = append(o.SuggestedActions.Descriptions, strings.Join(rmaMsgs, ", "))
		o.SuggestedActions.RepairActions = append(o.SuggestedActions.RepairActions, common.RepairActionTypeHardwareInspection)
	}

	return o
}

type Output struct {
	GPUProductName                    string                                         `json:"gpu_product_name"`
	MemoryErrorManagementCapabilities nvidia_query.MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities"`
	RemappedRowsNVML                  []nvidia_query_nvml.RemappedRows               `json:"remapped_rows_nvml"`

	// Recommended course of actions for any of the GPUs with a known issue.
	// For individual GPU details, see each per-GPU states.
	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameRemappedRows = "remapped_rows"

	StateKeyRemappedRowsData           = "data"
	StateKeyRemappedRowsEncoding       = "encoding"
	StateValueRemappedRowsEncodingJSON = "json"
)

func (o *Output) isRowRemappingSupported() bool {
	// even for "NVIDIA GeForce RTX 4090", this returns no error
	// thus "RemappedRowsNVML.Supported" is not a reliable way to check if row remapping is supported
	return o.MemoryErrorManagementCapabilities.RowRemapping
}

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate() (string, bool, error) {
	if o == nil {
		return "no data", true, nil
	}

	healthy := true
	reasons := []string{}

	if !o.isRowRemappingSupported() {
		reasons = append(reasons, fmt.Sprintf("GPU product name %q does not support row remapping (message: %q)", o.GPUProductName, o.MemoryErrorManagementCapabilities.Message))
	} else {
		for _, r := range o.RemappedRowsNVML {
			if r.QualifiesForRMA() {
				healthy = false
				reasons = append(reasons, fmt.Sprintf("GPU %s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", r.UUID, r.RemappedDueToUncorrectableErrors))
			}
			if r.RequiresReset() {
				healthy = false
				reasons = append(reasons, fmt.Sprintf("GPU %s needs reset (detected pending row remapping)", r.UUID))
			}
		}

		if len(reasons) == 0 {
			reasons = append(reasons, "no issue detected")
		}
	}

	reason := strings.Join(reasons, ", ")
	return reason, healthy, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameRemappedRows,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyRemappedRowsData:     string(b),
			StateKeyRemappedRowsEncoding: StateValueRemappedRowsEncodingJSON,
		},
	}

	if o.SuggestedActions != nil {
		log.Logger.Warnw("suggested actions", "suggestedActions", o.SuggestedActions.DescribeActions())
		state.SuggestedActions = o.SuggestedActions
	}

	return []components.State{state}, nil
}
