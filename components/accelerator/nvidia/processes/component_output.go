package processes

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
			o.Processes = append(o.Processes, device.Processes)
		}
	}
	return o
}

type Output struct {
	Processes []nvidia_query_nvml.Processes `json:"processes"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func (o *Output) YAML() ([]byte, error) {
	return yaml.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameProcesses = "processes"

	StateKeyProcessesData           = "data"
	StateKeyProcessesEncoding       = "encoding"
	StateValueProcessesEncodingJSON = "json"
)

func ParseStateProcesses(m map[string]string) (*Output, error) {
	data := m[StateKeyProcessesData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameProcesses:
			o, err := ParseStateProcesses(state.ExtraInfo)
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
	yb, _ := o.YAML()
	jb, _ := o.JSON()

	state := components.State{
		Name:    StateNameProcesses,
		Healthy: true,
		Reason:  string(yb),
		ExtraInfo: map[string]string{
			StateKeyProcessesData:     string(jb),
			StateKeyProcessesEncoding: StateValueProcessesEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
