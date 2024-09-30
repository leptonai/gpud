package remappedrows

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"
)

func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return nil
	}

	o := &Output{
		MemoryErrorManagementCapabilities: i.MemoryErrorManagementCapabilities,
	}

	if i.SMI != nil {
		for _, g := range i.SMI.GPUs {
			if g.RemappedRows == nil {
				continue
			}
			parsed, err := g.RemappedRows.Parse()
			if err != nil {
				log.Logger.Warnw("failed to parse temperature", "error", err)
				continue
			}
			o.RemappedRowsSMI = append(o.RemappedRowsSMI, parsed)

			rma, err := parsed.QualifiesForRMA()
			if err != nil {
				log.Logger.Warnw("failed to determine if GPU qualifies for RMA", "error", err)
				continue
			}
			if rma {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions, fmt.Sprintf("GPU %s qualifies for RMA (remapping failure occurred %v, remapped due to uncorrectable errors %s)", parsed.ID, parsed.RemappingFailed, parsed.RemappedDueToUncorrectableErrors))
			}

			requiresReset, err := parsed.RequiresReset()
			if err != nil {
				log.Logger.Warnw("failed to determine if GPU needs reset", "error", err)
				continue
			}
			if requiresReset {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.ResetGPU = true
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions, fmt.Sprintf("GPU %s needs reset (pending remapping %v)", parsed.ID, requiresReset))
			}
		}
	}

	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.RemappedRowsNVML = append(o.RemappedRowsNVML, device.RemappedRows)

			rma := device.RemappedRows.QualifiesForRMA()
			if rma {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions, fmt.Sprintf("GPU %s qualifies for RMA (remapping failure occurred %v)", device.UUID, device.RemappedRows.RemappingFailed))
			}

			requiresReset := device.RemappedRows.RequiresReset()
			if requiresReset {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.ResetGPU = true
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions, fmt.Sprintf("GPU %s needs reset (pending remapping %v)", device.UUID, requiresReset))
			}
		}
	}

	return o
}

type Output struct {
	MemoryErrorManagementCapabilities nvidia_query.MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities"`
	RemappedRowsSMI                   []nvidia_query.ParsedSMIRemappedRows           `json:"remapped_rows_smi"`
	RemappedRowsNVML                  []nvidia_query_nvml.RemappedRows               `json:"remapped_rows_nvml"`

	// Recommended course of actions for any of the GPUs with a known issue.
	// For individual GPU details, see each per-GPU states.
	RequiredActions *common.RequiredActions `json:"required_actions,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameRemappedRows = "remapped_rows"

	StateKeyRemappedRowsData           = "data"
	StateKeyRemappedRowsEncoding       = "encoding"
	StateValueRemappedRowsEncodingJSON = "json"
)

func ParseStateRemappedRows(m map[string]string) (*Output, error) {
	data := m[StateKeyRemappedRowsData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameRemappedRows:
			o, err := ParseStateRemappedRows(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			return o, nil

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, errors.New("no state found")
}

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate() (string, bool, error) {
	if o == nil {
		return "no data", true, nil
	}

	if !o.MemoryErrorManagementCapabilities.RowRemapping {
		return "row remapping is not supported", true, nil
	}

	healthy := true
	reasons := []string{}

	for _, r := range o.RemappedRowsSMI {
		rma, err := r.QualifiesForRMA()
		if err != nil {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvidia-smi GPU %s failed to determine if it qualifies for RMA: %s", r.ID, err.Error()))
			continue
		}
		if rma {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvidia-smi GPU %s qualifies for RMA (remapping failure occurred %v, remapped due to uncorrectable errors %s)", r.ID, r.RemappingFailed, r.RemappedDueToUncorrectableErrors))
		}

		needsReset, err := r.RequiresReset()
		if err != nil {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvidia-smi GPU %s failed to determine if it needs reset: %s", r.ID, err.Error()))
			continue
		}
		if needsReset {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvidia-smi GPU %s needs reset (pending remapping %v)", r.ID, needsReset))
		}
	}

	for _, r := range o.RemappedRowsNVML {
		if r.QualifiesForRMA() {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvml GPU %s qualifies for RMA (remapping failure occurred %v, remapped due to uncorrectable errors %d)", r.UUID, r.RemappingFailed, r.RemappedDueToUncorrectableErrors))
		}
		if r.RequiresReset() {
			healthy = false
			reasons = append(reasons, fmt.Sprintf("nvml GPU %s needs reset (pending remapping %v)", r.UUID, r.RemappingPending))
		}
	}

	reason := strings.Join(reasons, ", ")
	if len(reason) == 0 {
		reason = "no issues detected"
	}

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

	if o.RequiredActions != nil {
		state.RequiredActions = o.RequiredActions
	}

	return []components.State{state}, nil
}
