package processes

import (
	"encoding/json"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Data {
	if i == nil {
		return &Data{}
	}

	o := &Data{}

	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.Processes = append(o.Processes, device.Processes)
		}
	}

	return o
}

type Data struct {
	Processes []nvidia_query_nvml.Processes `json:"processes"`
}

func (d *Data) States() ([]components.State, error) {
	if d == nil {
		return nil, nil
	}

	data := ""
	if d != nil {
		b, _ := json.Marshal(d)
		data = string(b)
	}

	state := components.State{
		Name:    "processes",
		Healthy: true,
		Reason:  fmt.Sprintf("total %d processes", len(d.Processes)),
		ExtraInfo: map[string]string{
			"data":     data,
			"encoding": "json",
		},
	}
	return []components.State{state}, nil
}
