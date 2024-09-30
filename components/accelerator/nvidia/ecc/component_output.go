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
				o.VolatileUncorrectedErrors = append(o.VolatileUncorrectedErrors, fmt.Sprintf("[%s] %s", g.ID, strings.Join(errs, ", ")))
			}
		}
	}

	if i.NVML != nil {
		for _, dev := range i.NVML.DeviceInfos {
			o.ErrorCountsNVML = append(o.ErrorCountsNVML, dev.ECCErrors)

			if errs := dev.ECCErrors.Volatile.FindUncorrectedErrs(); len(errs) > 0 {
				o.VolatileUncorrectedErrors = append(o.VolatileUncorrectedErrors, fmt.Sprintf("[%s] %s", dev.UUID, strings.Join(errs, ", ")))
			}
		}
	}

	return o
}

type Output struct {
	ErrorCountsSMI  []nvidia_query.SMIECCErrors   `json:"error_counts_smi"`
	ErrorCountsNVML []nvidia_query_nvml.ECCErrors `json:"error_counts_nvml"`

	// Volatile counts are reset each time the driver loads.
	// As aggregate counts persist across reboots (i.e. for the lifetime of the device),
	// do not track separately.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g08978d1c4fb52b6a4c72b39de144f1d9
	//
	// A memory error that was not correctedFor ECC errors, these are double bit errors.
	// For Texture memory, these are errors where the resend fails.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
	VolatileUncorrectedErrors []string `json:"volatile_uncorrected_errors"`
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
	StateNameECCErrors = "ecc_errors"

	StateKeyECCErrorsData           = "data"
	StateKeyECCErrorsEncoding       = "encoding"
	StateValueECCErrorsEncodingJSON = "json"
)

func ParseStateECCErrors(m map[string]string) (*Output, error) {
	data := m[StateKeyECCErrorsData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameECCErrors:
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
	reasons := ""

	// as aggregate counts persist across reboots
	// ignore it for settings the healthy
	if len(o.VolatileUncorrectedErrors) > 0 {
		reasons = fmt.Sprintf("%d volatile errors found (from nvidia-smi and nvml): %s",
			len(o.VolatileUncorrectedErrors),
			strings.Join(o.VolatileUncorrectedErrors, ", "),
		)
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameECCErrors,
		Healthy: len(o.VolatileUncorrectedErrors) == 0,
		Reason:  reasons,
		ExtraInfo: map[string]string{
			StateKeyECCErrorsData:     string(b),
			StateKeyECCErrorsEncoding: StateValueECCErrorsEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
