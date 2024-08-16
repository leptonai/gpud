package peermem

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
)

func ToOutput(i *nvidia_query.Output) *Output {
	o := &Output{
		LsmodPeermem: *i.LsmodPeermem,
	}
	if len(i.SMI.GPUs) > 0 {
		o.ProductName = i.SMI.GPUs[0].ProductName
	}
	return o
}

type Output struct {
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
		Name:    StateNameLsmodPeermem,
		Healthy: true,
		Reason:  fmt.Sprintf("ibcore is using peermem module? %v", o.LsmodPeermem.IbcoreUsingPeermemModule),
		ExtraInfo: map[string]string{
			StateKeyLsmodPeermemData:     string(b),
			StateKeyLsmodPeermemEncoding: StateValueLsmodPeermemEncodingJSON,
		},
	}
	if nvidia_query.IsIbcoreExpected(
		o.ProductName,
		o.LsmodPeermem.IbstatExists,
		o.LsmodPeermem.InfinibandClassExists,
	) &&
		!o.LsmodPeermem.IbcoreUsingPeermemModule {
		state.Healthy = false
		state.Reason = "ibcore is not using peermem module"
	}
	return []components.State{state}, nil
}
