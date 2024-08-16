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

func ToOutput(i *nvidia_query.Output) *Output {
	clockEvents := make([]nvidia_query_nvml.ClockEvents, len(i.NVML.DeviceInfos))
	for idx, devInfo := range i.NVML.DeviceInfos {
		clockEvents[idx] = devInfo.ClockEvents
	}
	return &Output{
		HWSlowdownSMI: HWSlowdownSMI{
			Errors: i.SMI.FindHWSlowdownErrs(),
		},
		ClockEventsNVML: clockEvents,
	}
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

	hasClockEventError := false
	reasons := []string{}
	for _, clockEvents := range o.ClockEventsNVML {
		if len(clockEvents.Reasons) == 0 {
			continue
		}
		if clockEvents.HWSlowdown {
			hasClockEventError = true
			reasons = append(reasons, clockEvents.UUID+" hw slowdown")
			break
		}
		if clockEvents.HWSlowdownThermal {
			hasClockEventError = true
			reasons = append(reasons, clockEvents.UUID+" hw slowdown thermal")
			break
		}
		if clockEvents.HWSlowdownPowerBrake {
			hasClockEventError = true
			reasons = append(reasons, clockEvents.UUID+" hw slowdown power brake")
			break
		}
	}

	yb, err := yaml.Marshal(reasons)
	if err != nil {
		return nil, err
	}

	if !hasClockEventError && len(o.HWSlowdownSMI.Errors) == 0 {
		rm := "no critical clock event error found"
		if len(reasons) > 0 {
			rm = "\n\n(below are other non-critical reasons found)\n\n" + string(yb)
		}
		return []components.State{
			{
				Name:    StateNameHWSlowdown,
				Healthy: true,
				Error:   nil,
				Reason:  rm,
				ExtraInfo: map[string]string{
					StateKeyHWSlowdownData:     string(b),
					StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
				},
			},
		}, nil
	}

	healthy := !hasClockEventError && len(o.HWSlowdownSMI.Errors) == 0

	return []components.State{
		{
			Name:    StateNameHWSlowdown,
			Healthy: healthy,
			Reason:  "clock event found\n\n" + string(yb),
			ExtraInfo: map[string]string{
				StateKeyHWSlowdownData:     string(b),
				StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
			},
		},
	}, nil
}
