package infiniband

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
		InfinibandClassExists: i.InfinibandClassExists,
		IbstatExists:          i.IbstatExists,
	}
	if i.Ibstat != nil {
		o.Ibstat = *i.Ibstat
	}

	return o
}

type Output struct {
	InfinibandClassExists bool                      `json:"infiniband_class_exists"`
	IbstatExists          bool                      `json:"ibstat_exists"`
	Ibstat                nvidia_query.IbstatOutput `json:"ibstat"`
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
	StateNameIbstat = "ibstat"

	StateKeyIbstatData           = "data"
	StateKeyIbstatEncoding       = "encoding"
	StateValueIbstatEncodingJSON = "json"
)

func ParseStateIbstat(m map[string]string) (*Output, error) {
	o := &Output{}
	data := m[StateKeyIbstatData]
	if err := json.Unmarshal([]byte(data), &o.Ibstat); err != nil {
		return nil, err
	}
	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameIbstat:
			o, err := ParseStateIbstat(state.ExtraInfo)
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
	if len(o.Ibstat.Errors) > 0 {
		return fmt.Sprintf("ibstat errors found: %s", strings.Join(o.Ibstat.Errors, ", ")), false, nil
	}
	return "no ibstat error found", true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameIbstat,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyIbstatData:     string(b),
			StateKeyIbstatEncoding: StateValueIbstatEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
