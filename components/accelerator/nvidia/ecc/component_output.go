package ecc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{}

	if i.SMI != nil {
		for _, g := range i.SMI.GPUs {
			if g.ECCErrors == nil {
				continue
			}

			o.ErrorCountsSMI = append(o.ErrorCountsSMI, *g.ECCErrors)

			if errs := g.ECCErrors.FindVolatileUncorrectableErrs(); len(errs) > 0 {
				o.VolatileUncorrectedErrorsFromSMI = append(o.VolatileUncorrectedErrorsFromSMI, fmt.Sprintf("[%s] %s", g.ID, strings.Join(errs, ", ")))
			}
		}
	}

	if i.NVML != nil {
		for _, dev := range i.NVML.DeviceInfos {
			o.ECCModes = append(o.ECCModes, dev.ECCMode)
			o.ErrorCountsNVML = append(o.ErrorCountsNVML, dev.ECCErrors)

			if errs := dev.ECCErrors.Volatile.FindUncorrectedErrs(); len(errs) > 0 {
				o.VolatileUncorrectedErrorsFromNVML = append(o.VolatileUncorrectedErrorsFromNVML, fmt.Sprintf("[%s] %s", dev.UUID, strings.Join(errs, ", ")))
			}
		}
	}

	return o
}

type Output struct {
	ECCModes []nvidia_query_nvml.ECCMode `json:"ecc_modes"`

	ErrorCountsSMI  []nvidia_query.SMIECCErrors   `json:"error_counts_smi"`
	ErrorCountsNVML []nvidia_query_nvml.ECCErrors `json:"error_counts_nvml"`

	// Volatile counts are reset each time the driver loads.
	// As aggregate counts persist across reboots (i.e. for the lifetime of the device),
	// we do not track separately.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g08978d1c4fb52b6a4c72b39de144f1d9
	//
	// A memory error that was not corrected.
	// For ECC errors, these are double bit errors.
	// For Texture memory, these are errors where the resend fails.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
	VolatileUncorrectedErrorsFromSMI  []string `json:"volatile_uncorrected_errors_from_smi"`
	VolatileUncorrectedErrorsFromNVML []string `json:"volatile_uncorrected_errors_from_nvml"`
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
	StateNameECC = "ecc"

	StateKeyECCData           = "data"
	StateKeyECCEncoding       = "encoding"
	StateValueECCEncodingJSON = "json"
)

func ParseStateECCErrors(m map[string]string) (*Output, error) {
	data := m[StateKeyECCData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameECC:
			o, err := ParseStateECCErrors(state.ExtraInfo)
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

func (o *Output) States() ([]components.State, error) {
	reasons := []string{}

	// as aggregate counts persist across reboots
	// ignore it for settings the healthy
	if len(o.VolatileUncorrectedErrorsFromSMI) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d volatile errors found (from nvidia-smi): %s",
			len(o.VolatileUncorrectedErrorsFromSMI),
			strings.Join(o.VolatileUncorrectedErrorsFromSMI, ", "),
		))
	}

	if len(o.VolatileUncorrectedErrorsFromNVML) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d volatile errors found (from nvml): %s",
			len(o.VolatileUncorrectedErrorsFromNVML),
			strings.Join(o.VolatileUncorrectedErrorsFromNVML, ", "),
		))
	}

	reason := strings.Join(reasons, "; ")
	if len(reason) == 0 {
		reason = "no issue detected"
	} else {
		reason = fmt.Sprintf("note that when an uncorrectable ECC error is detected, the NVIDIA driver software will perform error recovery -- details of ecc status are: %s", reason)
	}

	b, _ := o.JSON()
	state := components.State{
		Name: StateNameECC,

		// no reason to mark this unhealthy as "when an uncorrectable ECC error is detected, the NVIDIA driver software will perform error recovery."
		// we only mark this unhealthy when the pending row remapping is >0 (which requires GPU reset)
		// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html
		Healthy: true,

		Reason: reason,
		ExtraInfo: map[string]string{
			StateKeyECCData:     string(b),
			StateKeyECCEncoding: StateValueECCEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
