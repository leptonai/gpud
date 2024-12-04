package infiniband

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate(cfg Config) (string, bool, error) {
	if !infiniband.SupportsInfinibandProduct(o.GPUProductName) {
		return fmt.Sprintf("%q GPUs do not support infiniband", o.GPUProductName), true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	count, err := infiniband.CountInfinibandPCIBuses(ctx)
	if err != nil {
		return fmt.Sprintf("failed to count infiniband pci buses: %s", err), false, nil
	}
	if count == 0 {
		return "no infiniband pci buses found", true, nil
	}

	if o.InfinibandClassExists && o.IbstatExists {
		if len(o.Ibstat.Errors) > 0 {
			return fmt.Sprintf("infiniband suppported but ibstat errors found: %s", strings.Join(o.Ibstat.Errors, ", ")), false, nil
		}
		if len(o.Ibstat.Parsed) > 0 {
			// no port count is set, use the gpu count as port count
			expectedPortCount := cfg.ExpectedPortStates.PortCount

			// some H100 machines only have 1 ib port in ib class dir
			if expectedPortCount == 0 {
				expectedPortCount = infiniband.CountInfinibandClass()
			}

			// H100 machines with 12 ib ports should default to the GPU count 8
			if expectedPortCount == 0 || expectedPortCount > o.GPUCount {
				expectedPortCount = o.GPUCount
			}

			// no rate is set, use the default rate based on the product
			expectedRate := cfg.ExpectedPortStates.Rate
			if expectedRate == 0 {
				expectedRate = infiniband.SupportsInfinibandPortRate(o.GPUProductName)
			}

			matched := o.Ibstat.Parsed.Count("LinkUp", "Active", expectedRate)
			if matched < expectedPortCount {
				return fmt.Sprintf("only %d out of %d ibstat cards are active and link up (expected rate: %d Gb/sec)", matched, expectedPortCount, expectedRate), false, nil
			}
		}
	}
	return "no infiniband class found or no ibstat exists or no ibstat error found", true, nil
}

func (o *Output) States(cfg Config) ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate(cfg)
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
