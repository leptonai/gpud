package utilization

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
			o.Utilizations = append(o.Utilizations, device.Utilization)
		}
	}

	return o
}

type Output struct {
	Utilizations []nvidia_query_nvml.Utilization `json:"utilizations"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameUtilization = "utilization"

	StateKeyUtilizationData           = "data"
	StateKeyUtilizationEncoding       = "encoding"
	StateValueUtilizationEncodingJSON = "json"
)

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate() (string, bool, error) {
	yb, err := yaml.Marshal(o.Utilizations)
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
