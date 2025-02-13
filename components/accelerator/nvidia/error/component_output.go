package error

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"

	"sigs.k8s.io/yaml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	if i.SMI == nil {
		return &Output{}
	}

	return &Output{
		Errors: i.SMI.FindGPUErrs(),
	}
}

type Output struct {
	Errors []string `json:"errors"`
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
	StateNameError = "error"

	StateKeyErrorData           = "data"
	StateKeyErrorEncoding       = "encoding"
	StateValueErrorEncodingJSON = "json"
)

func ParseStateError(m map[string]string) (*Output, error) {
	data := m[StateKeyErrorData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameError:
			o, err := ParseStateError(state.ExtraInfo)
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
	if len(o.Errors) == 0 {
		return "no gpu error found", true, nil
	}
	yb, err := yaml.Marshal(o.Errors)
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("gpu error found:\n\n%s\n", string(yb)), false, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameError,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyErrorData:     string(b),
			StateKeyErrorEncoding: StateValueErrorEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
