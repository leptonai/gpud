package nvlink

import (
	"encoding/json"
	"errors"
	"fmt"

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
			o.NVLinkDevices = append(o.NVLinkDevices, device.NVLink)
		}
	}

	return o
}

type Output struct {
	NVLinkDevices []nvidia_query_nvml.NVLink `json:"nvlink_devices"`
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
	StateNameNVLinkDevices = "nvlink_devices"

	StateKeyNVLinkDevicesData           = "data"
	StateKeyNVLinkDevicesEncoding       = "encoding"
	StateValueNVLinkDevicesEncodingJSON = "json"
)

func ParseStateNVLinkDevices(m map[string]string) (*Output, error) {
	data := m[StateKeyNVLinkDevicesData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameNVLinkDevices:
			o, err := ParseStateNVLinkDevices(state.ExtraInfo)
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
	reason := fmt.Sprintf("%d GPU(s):", len(o.NVLinkDevices))

	// iterate all links per GPU and sum all the errors
	for _, device := range o.NVLinkDevices {
		allCRCErrs := uint64(0)
		allRelayErrs := uint64(0)
		allRecErrs := uint64(0)
		for _, link := range device.States {
			allCRCErrs += link.CRCErrors
			allRelayErrs += link.ReplayErrors
			allRecErrs += link.RecoveryErrors
		}
		reason += fmt.Sprintf("\n- %s: %d crc, %d relay, %d recovery errors (total %d links)", device.UUID, allCRCErrs, allRelayErrs, allRecErrs, len(device.States))
	}

	return reason, true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameNVLinkDevices,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyNVLinkDevicesData:     string(b),
			StateKeyNVLinkDevicesEncoding: StateValueNVLinkDevicesEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
