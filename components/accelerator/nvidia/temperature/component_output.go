package temperature

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	"github.com/leptonai/gpud/log"

	"sigs.k8s.io/yaml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{}

	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.UsagesNVML = append(o.UsagesNVML, device.Temperature)
		}
	}

	if i.SMI != nil {
		for _, g := range i.SMI.GPUs {
			if g.Temperature == nil {
				continue
			}
			parsed, err := g.Temperature.Parse()
			if err != nil {
				log.Logger.Warnw("failed to parse temperature", "error", err)
				continue
			}
			o.UsagesSMI = append(o.UsagesSMI, parsed)
		}
	}

	return o
}

type Output struct {
	UsagesSMI  []nvidia_query.ParsedTemperature `json:"usages_smi"`
	UsagesNVML []nvidia_query_nvml.Temperature  `json:"usages_nvml"`
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
	StateNameTemperature = "temperature"

	StateKeyTemperatureData           = "data"
	StateKeyTemperatureEncoding       = "encoding"
	StateValueTemperatureEncodingJSON = "json"
)

func ParseStateTemperature(m map[string]string) (*Output, error) {
	data := m[StateKeyTemperatureData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameTemperature:
			o, err := ParseStateTemperature(state.ExtraInfo)
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
	type temp struct {
		UUID        string `json:"uuid"`
		Limit       uint32 `json:"limit"`
		Usage       uint32 `json:"usage"`
		UsedPercent string `json:"used_percent"`
	}

	memThresholdExceeded := []string{}
	ts := make([]temp, len(o.UsagesNVML))
	for i, u := range o.UsagesNVML {
		// same logic as DCGM "VerifyHBMTemperature" that alerts  "DCGM_FR_TEMP_VIOLATION",
		// use "DCGM_FI_DEV_MEM_MAX_OP_TEMP" to get the max HBM temperature threshold "NVML_TEMPERATURE_THRESHOLD_MEM_MAX"
		if u.ThresholdCelsiusMemMax > 0 && u.CurrentCelsiusGPUCore > u.ThresholdCelsiusMemMax {
			memThresholdExceeded = append(memThresholdExceeded,
				fmt.Sprintf("%s current temperature is %d °C exceeding the HBM temperature threshold %d °C",
					u.UUID,
					u.CurrentCelsiusGPUCore,
					u.ThresholdCelsiusMemMax,
				),
			)
		}

		ts[i] = temp{
			UUID:        u.UUID,
			Limit:       u.ThresholdCelsiusSlowdown,
			Usage:       u.CurrentCelsiusGPUCore,
			UsedPercent: u.UsedPercentSlowdown,
		}
	}

	if len(memThresholdExceeded) > 0 {
		return strings.Join(memThresholdExceeded, ", "), false, nil
	}

	yb, err := yaml.Marshal(ts)
	if err != nil {
		return "", false, err
	}
	return string(yb), true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameTemperature,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyTemperatureData:     string(b),
			StateKeyTemperatureEncoding: StateValueTemperatureEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
