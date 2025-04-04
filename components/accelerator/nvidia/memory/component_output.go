package memory

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
			o.UsagesNVML = append(o.UsagesNVML, device.Memory)
		}
	}
	return o
}

type Output struct {
	UsagesNVML []nvidia_query_nvml.Memory `json:"usages_nvml"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameMemoryUsage = "memory_usage"

	StateKeyMemoryUsageData           = "data"
	StateKeyMemoryUsageEncoding       = "encoding"
	StateValueMemoryUsageEncodingJSON = "json"
)

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate() (string, bool, error) {
	type mem struct {
		UUID        string `json:"uuid"`
		TotalBytes  string `json:"total_bytes"`
		UsedBytes   string `json:"used_bytes"`
		UsedPercent string `json:"used_percent"`
	}
	mems := make([]mem, len(o.UsagesNVML))
	for i, u := range o.UsagesNVML {
		mems[i] = mem{
			UUID:        u.UUID,
			TotalBytes:  u.TotalHumanized,
			UsedBytes:   u.UsedHumanized,
			UsedPercent: u.UsedPercent,
		}
	}
	yb, err := yaml.Marshal(mems)
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
		Name:    StateNameMemoryUsage,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyMemoryUsageData:     string(b),
			StateKeyMemoryUsageEncoding: StateValueMemoryUsageEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
