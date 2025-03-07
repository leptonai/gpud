package processes

import (
	"encoding/json"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

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

const (
	StateNameProcesses = "processes"

	StateKeyProcessesData           = "data"
	StateKeyProcessesEncoding       = "encoding"
	StateValueProcessesEncodingJSON = "json"
)

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
