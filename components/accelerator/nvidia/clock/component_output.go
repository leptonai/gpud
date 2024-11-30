package clock

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"

	"sigs.k8s.io/yaml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{}

	if i.NVML != nil {
		for _, devInfo := range i.NVML.DeviceInfos {
			if devInfo.ClockEvents != nil {
				o.ClockEventsNVML = append(o.ClockEventsNVML, *devInfo.ClockEvents)
			}
		}
	}

	if i.SMI != nil {
		o.HWSlowdownSMI = HWSlowdownSMI{
			Errors: i.SMI.FindHWSlowdownErrs(),
		}
	}

	return o
}

type Output struct {
	HWSlowdownSMI   HWSlowdownSMI                   `json:"hw_slowdown_smi"`
	ClockEventsNVML []nvidia_query_nvml.ClockEvents `json:"clock_events_nvml"`
}

type HWSlowdownSMI struct {
	Errors []string `json:"errors"`
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
	StateNameHWSlowdown = "hw_slowdown"

	StateKeyHWSlowdownData           = "data"
	StateKeyHWSlowdownEncoding       = "encoding"
	StateValueHWSlowdownEncodingJSON = "json"
)

func ParseStateHWSlowdown(m map[string]string) (*Output, error) {
	data := m[StateKeyHWSlowdownData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameHWSlowdown:
			o, err := ParseStateHWSlowdown(state.ExtraInfo)
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
	b, _ := o.JSON()

	clockEventsReasons := make([]string, 0)
	for _, clockEvents := range o.ClockEventsNVML {
		if len(clockEvents.Reasons) > 0 {
			clockEventsReasons = append(clockEventsReasons, clockEvents.Reasons...)
		}
		if clockEvents.HWSlowdown {
			clockEventsReasons = append(clockEventsReasons, clockEvents.UUID+" hw slowdown (nvml)")
		}
		if clockEvents.HWSlowdownThermal {
			clockEventsReasons = append(clockEventsReasons, clockEvents.UUID+" hw slowdown thermal (nvml)")
		}
		if clockEvents.HWSlowdownPowerBrake {
			clockEventsReasons = append(clockEventsReasons, clockEvents.UUID+" hw slowdown power brake (nvml)")
		}
	}
	if len(o.HWSlowdownSMI.Errors) > 0 {
		clockEventsReasons = append(clockEventsReasons, o.HWSlowdownSMI.Errors...)
	}

	if len(clockEventsReasons) == 0 {
		return []components.State{
			{
				Name:    StateNameHWSlowdown,
				Healthy: true,
				Reason:  "no critical clock event error found (nvml or nvidia-smi)",
				ExtraInfo: map[string]string{
					StateKeyHWSlowdownData:     string(b),
					StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
				},
			},
		}, nil
	}

	yb, err := yaml.Marshal(clockEventsReasons)
	if err != nil {
		return nil, err
	}
	return []components.State{
		{
			Name:    StateNameHWSlowdown,
			Healthy: false,
			Reason:  "clock events found\n\n" + string(yb),
			ExtraInfo: map[string]string{
				StateKeyHWSlowdownData:     string(b),
				StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
			},
		},
	}, nil
}
