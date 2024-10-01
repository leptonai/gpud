package sxid

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/leptonai/gpud/components"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	"github.com/leptonai/gpud/components/common"

	"sigs.k8s.io/yaml"
)

type Output struct {
	DmesgErrors []nvidia_query_sxid.DmesgError `json:"dmesg_errors,omitempty"`

	// Recommended course of actions for any of the GPUs with a known issue.
	// For individual GPU details, see each per-GPU states.
	// Used for states calls.
	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`

	// Used for events calls.
	SuggestedActionsPerLogLine map[string]*common.SuggestedActions `json:"suggested_actions_per_log_line,omitempty"`
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

func (o *Output) YAML() ([]byte, error) {
	return yaml.Marshal(o)
}

func ParseOutputYAML(data []byte) (*Output, error) {
	o := new(Output)
	if err := yaml.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameErrorSXid = "error_sxid"

	StateKeyErrorSXidData           = "data"
	StateKeyErrorSXidEncoding       = "encoding"
	StateValueErrorSXidEncodingJSON = "json"
)

func ParseStateErrorSXid(m map[string]string) (*Output, error) {
	data := m[StateKeyErrorSXidData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameErrorSXid:
			o, err := ParseStateErrorSXid(state.ExtraInfo)
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
	if len(o.DmesgErrors) == 0 {
		return "no sxid error found", true, nil
	}
	yb, err := yaml.Marshal(o.DmesgErrors)
	if err != nil {
		return "", false, err
	}
	return "sxid error found from dmesg\n\n" + string(yb), false, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameErrorSXid,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyErrorSXidData:     string(b),
			StateKeyErrorSXidEncoding: StateValueErrorSXidEncodingJSON,
		},
	}

	if o.SuggestedActions != nil {
		state.RequiredActions = o.SuggestedActions
	}

	return []components.State{state}, nil
}

const (
	EventNameErroSXid = "error_sxid"

	EventKeyErroSXidUnixSeconds    = "unix_seconds"
	EventKeyErroSXidData           = "data"
	EventKeyErroSXidEncoding       = "encoding"
	EventValueErroSXidEncodingJSON = "json"
)

func (o *Output) Events() []components.Event {
	des := make([]components.Event, 0)
	for _, de := range o.DmesgErrors {
		b, _ := de.JSON()

		var actions *common.SuggestedActions = nil
		if o.SuggestedActionsPerLogLine != nil {
			actions = o.SuggestedActionsPerLogLine[de.LogItem.Line]
		}

		des = append(des, components.Event{
			Time: de.LogItem.Time,
			Name: EventNameErroSXid,
			ExtraInfo: map[string]string{
				EventKeyErroSXidUnixSeconds: strconv.FormatInt(de.LogItem.Time.Unix(), 10),
				EventKeyErroSXidData:        string(b),
				EventKeyErroSXidEncoding:    StateValueErrorSXidEncodingJSON,
			},
			RequiredActions: actions,
		})
	}
	if len(des) == 0 {
		return nil
	}
	return des
}
