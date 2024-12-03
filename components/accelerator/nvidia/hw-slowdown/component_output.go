package hwslowdown

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

	if i.NVML != nil {
		for _, devInfo := range i.NVML.DeviceInfos {
			if devInfo.ClockEvents != nil {
				o.HWSlowdownEventsNVML = append(o.HWSlowdownEventsNVML, *devInfo.ClockEvents)
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
	HWSlowdownEventsNVML []nvidia_query_nvml.ClockEvents `json:"hw_slowdown_events_nvml"`
	HWSlowdownSMI        HWSlowdownSMI                   `json:"hw_slowdown_smi"`
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

	reasons := make([]string, 0)

	// skip "o.HWSlowdownEventsNVML" since this will be returned in events
	// use nvidia-smi as a fallback
	// TODO: remove this once we have confirmed that events via NVML works well
	// to detect hardware slowdown
	if len(o.HWSlowdownSMI.Errors) > 0 {
		reasons = append(reasons, o.HWSlowdownSMI.Errors...)
	}

	if len(reasons) == 0 {
		return []components.State{
			{
				Name:    StateNameHWSlowdown,
				Healthy: true,
				Reason:  "no hardware slowdown found in nvidia-smi",
				ExtraInfo: map[string]string{
					StateKeyHWSlowdownData:     string(b),
					StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
				},
			},
		}, nil
	}

	return []components.State{
		{
			Name:    StateNameHWSlowdown,
			Healthy: false,
			Reason:  "hw slowdown found in nvidia-smi: " + strings.Join(reasons, ", "),
			ExtraInfo: map[string]string{
				StateKeyHWSlowdownData:     string(b),
				StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
			},
		},
	}, nil
}
