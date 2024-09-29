package peermem

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{
		LsmodPeermem: *i.LsmodPeermem,
		GPUCounts:    i.GPUCounts(),
		ProductName:  i.GPUProductName(),
	}
	return o
}

type Output struct {
	// Represents the number of GPUs in the system.
	// This is used to determine if ibcore may be expected to use peermem module.
	GPUCounts int `json:"gpu_counts"`

	ProductName  string                                `json:"product_name"`
	LsmodPeermem nvidia_query.LsmodPeermemModuleOutput `json:"lsmod_peermem"`
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
	StateNameLsmodPeermem = "lsmod_peermem"

	StateKeyLsmodPeermemData           = "data"
	StateKeyLsmodPeermemEncoding       = "encoding"
	StateValueLsmodPeermemEncodingJSON = "json"

	// TODO: support compressed gzip
)

func ParseStateLsmodPeermem(m map[string]string) (*Output, error) {
	o := &Output{}
	data := m[StateKeyLsmodPeermemData]
	if err := json.Unmarshal([]byte(data), &o.LsmodPeermem); err != nil {
		return nil, err
	}
	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameLsmodPeermem:
			o, err := ParseStateLsmodPeermem(state.ExtraInfo)
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
	state := components.State{
		Name: StateNameLsmodPeermem,

		// the peermem module depends on each machine setup
		// so we don't decide whether peermem is required or not
		Healthy: true,

		Reason: fmt.Sprintf("ibcore is using peermem module? %v (gpu counts: %d)", o.LsmodPeermem.IbcoreUsingPeermemModule, o.GPUCounts),
		ExtraInfo: map[string]string{
			StateKeyLsmodPeermemData:     string(b),
			StateKeyLsmodPeermemEncoding: StateValueLsmodPeermemEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
