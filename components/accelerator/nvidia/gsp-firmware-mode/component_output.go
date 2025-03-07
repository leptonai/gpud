package gspfirmwaremode

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
		for _, device := range i.NVML.DeviceInfos {
			o.GSPFirmwareModesNVML = append(o.GSPFirmwareModesNVML, device.GSPFirmwareMode)
		}
	}

	return o
}

type Output struct {
	GSPFirmwareModesNVML []nvidia_query_nvml.GSPFirmwareMode `json:"gsp_firmware_modes_nvml"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameGSPFirmwareMode = "gsp_firmware_mode"

	StateKeyGSPFirmwareModeData       = "data"
	StateKeyGSPFirmwareModeEncoding   = "encoding"
	StateValueMemoryUsageEncodingJSON = "json"
)

func (o *Output) States() ([]components.State, error) {
	reasons := []string{}
	for _, mode := range o.GSPFirmwareModesNVML {
		if !mode.Enabled {
			reasons = append(reasons, fmt.Sprintf("device %s does not enable GSP firmware mode (GSP mode supported: %v)", mode.UUID, mode.Supported))
		}
	}
	reason := "GSP firmware mode is disabled for all devices"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameGSPFirmwareMode,
		Healthy: true,
		Reason:  reason,
		ExtraInfo: map[string]string{
			StateKeyGSPFirmwareModeData:     string(b),
			StateKeyGSPFirmwareModeEncoding: StateValueMemoryUsageEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
