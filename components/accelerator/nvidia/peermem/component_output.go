package peermem

import (
	"encoding/json"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/peermem"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{
		LsmodPeermem: *i.LsmodPeermem,
		GPUCount:     i.GPUCount(),
		ProductName:  i.GPUProductName(),
	}
	return o
}

type Output struct {
	// Represents the number of GPUs in the system.
	// This is used to determine if ibcore may be expected to use peermem module.
	GPUCount int `json:"gpu_count"`

	ProductName  string                           `json:"product_name"`
	LsmodPeermem peermem.LsmodPeermemModuleOutput `json:"lsmod_peermem"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameLsmodPeermem = "lsmod_peermem"

	StateKeyLsmodPeermemData           = "data"
	StateKeyLsmodPeermemEncoding       = "encoding"
	StateValueLsmodPeermemEncodingJSON = "json"

	// TODO: support compressed gzip
)

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	state := components.State{
		Name: StateNameLsmodPeermem,

		// the peermem module depends on each machine setup
		// so we don't decide whether peermem is required or not
		Healthy: true,

		Reason: fmt.Sprintf("ibcore is using peermem module? %v (gpu counts: %d)", o.LsmodPeermem.IbcoreUsingPeermemModule, o.GPUCount),
		ExtraInfo: map[string]string{
			StateKeyLsmodPeermemData:     string(b),
			StateKeyLsmodPeermemEncoding: StateValueLsmodPeermemEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
