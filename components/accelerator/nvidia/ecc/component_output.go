package ecc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{}

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
	VolatileUncorrectedErrorsFromNVML []string `json:"volatile_uncorrected_errors_from_nvml"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameECC = "ecc"

	StateKeyECCData           = "data"
	StateKeyECCEncoding       = "encoding"
	StateValueECCEncodingJSON = "json"
)

func (o *Output) States() ([]components.State, error) {
	reasons := []string{}

	if len(o.VolatileUncorrectedErrorsFromNVML) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d volatile errors found (from nvml)",
			len(o.VolatileUncorrectedErrorsFromNVML),
		))
	}

	reason := strings.Join(reasons, "; ")
	if len(reason) == 0 {
		reason = "no issue detected"
	} else {
		reason = fmt.Sprintf("note that when an uncorrectable ECC error is detected, the NVIDIA driver software will perform error recovery -- %s", reason)
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
