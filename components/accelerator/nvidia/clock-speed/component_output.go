package clockspeed

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
	o := &Output{}
	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.ClockSpeeds = append(o.ClockSpeeds, device.ClockSpeed)
		}
	}
	return o
}

type Output struct {
	ClockSpeeds []nvidia_query_nvml.ClockSpeed `json:"clock_speeds"`
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
	StateNameUtilization = "clock_speed"

	StateKeyUtilizationData           = "data"
	StateKeyUtilizationEncoding       = "encoding"
	StateValueUtilizationEncodingJSON = "json"
)

func ParseStateClockSpeed(m map[string]string) (*Output, error) {
	data := m[StateKeyUtilizationData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameUtilization:
			o, err := ParseStateClockSpeed(state.ExtraInfo)
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
	yb, err := yaml.Marshal(o.ClockSpeeds)
	if err != nil {
		return "", false, err
	}
	return string(yb), true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameUtilization,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyUtilizationData:     string(b),
			StateKeyUtilizationEncoding: StateValueUtilizationEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
