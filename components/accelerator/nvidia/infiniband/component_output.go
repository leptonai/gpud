package infiniband

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	"github.com/leptonai/gpud/components/common"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{
		GPUProductName:        i.GPUProductName(),
		GPUCount:              i.GPUCount(),
		InfinibandClassExists: i.InfinibandClassExists,
		IbstatExists:          i.IbstatExists,
	}
	if i.Ibstat != nil {
		o.Ibstat = *i.Ibstat
	}

	return o
}

type Output struct {
	// GPUProductName is the product name of the GPU.
	// Useful to ignore infiniband states for non-infiniband supported GPUs (e.g., GTX 4090).
	GPUProductName string `json:"gpu_product_name"`

	// Represents the number of GPUs in the system.
	// This is used to determine how many ibstat cards at certain rate are expected.
	GPUCount int `json:"gpu_count"`

	InfinibandClassExists bool                    `json:"infiniband_class_exists"`
	IbstatExists          bool                    `json:"ibstat_exists"`
	Ibstat                infiniband.IbstatOutput `json:"ibstat"`
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

var (
	msgThresholdNotSetSkipped = "ports or rate threshold not set, skipping"
	msgNoIbstatExists         = "no ibstat exists while configured to check ibstat"
	msgNoIbstatDataFound      = "no ibstat data found while configured to check ibstat"
	msgNoIbstatIssueFound     = "no infiniband issue found in ibstat"
)

// Returns the output evaluation reason and its healthy-ness.
// We DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
func (o *Output) Evaluate(cfg ExpectedPortStates) (string, bool, error) {
	// nothing specified for this machine, gpud MUST skip the ib check
	if cfg.AtLeastPorts <= 0 && cfg.AtLeastRate <= 0 {
		return msgThresholdNotSetSkipped, true, nil
	}

	if len(o.Ibstat.Errors) > 0 {
		return fmt.Sprintf("ibstat errors found: %s", strings.Join(o.Ibstat.Errors, ", ")), false, nil
	}

	if !o.IbstatExists {
		return msgNoIbstatExists, false, nil
	}

	if len(o.Ibstat.Parsed) == 0 {
		return msgNoIbstatDataFound, false, nil
	}

	atLeastPorts := cfg.AtLeastPorts
	atLeastRate := cfg.AtLeastRate
	if err := o.Ibstat.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate); err != nil {
		return err.Error(), false, nil
	}

	return msgNoIbstatIssueFound, true, nil
}

func (o *Output) States(cfg Config) ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate(GetDefaultExpectedPortStates())
	if err != nil {
		return nil, err
	}

	b, _ := o.JSON()

	var suggestedActions *common.SuggestedActions = nil
	if !healthy {
		suggestedActions = &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeHardwareInspection,
			},
			Descriptions: []string{
				"potential infiniband switch/hardware issue needs immediate attention",
			},
		}
	}

	state := components.State{
		Name: StateNameIbstat,

		Healthy: healthy,
		Reason:  outputReasons,

		ExtraInfo: map[string]string{
			nvidia_query.StateKeyGPUProductName: o.GPUProductName,
			nvidia_query.StateKeyIbstatExists:   fmt.Sprintf("%v", o.IbstatExists),
			StateKeyIbstatData:                  string(b),
			StateKeyIbstatEncoding:              StateValueIbstatEncodingJSON,
		},

		SuggestedActions: suggestedActions,
	}
	return []components.State{state}, nil
}
