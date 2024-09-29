package fabricmanager

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

	o := &Output{}

	if i.FabricManager != nil {
		o.FabricManager = *i.FabricManager
	}

	return o
}

type Output struct {
	FabricManager nvidia_query.FabricManagerOutput `json:"fabric_manager"`
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
	StateNameFabricManager = "fabric_manager"

	StateKeyFabricManagerData           = "data"
	StateKeyFabricManagerEncoding       = "encoding"
	StateValueFabricManagerEncodingJSON = "json"

	// TODO: support compressed gzip
)

func ParseStateFabricManager(m map[string]string) (*Output, error) {
	o := &Output{}
	data := m[StateKeyFabricManagerData]
	if err := json.Unmarshal([]byte(data), &o.FabricManager); err != nil {
		return nil, err
	}
	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameFabricManager:
			o, err := ParseStateFabricManager(state.ExtraInfo)
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
	if o.FabricManager.Active {
		return "fabric-manager active", true, nil
	}
	return "fabric-manager inactive", true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameFabricManager,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyFabricManagerData:     string(b),
			StateKeyFabricManagerEncoding: StateValueFabricManagerEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
