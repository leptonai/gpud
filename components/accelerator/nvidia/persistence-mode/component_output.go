package persistencemode

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{
		PersistencedExists:  i.PersistencedExists,
		PersistencedRunning: i.PersistencedRunning,
	}

	if i.SMI != nil {
		for _, g := range i.SMI.GPUs {
			o.PersistenceModesSMI = append(o.PersistenceModesSMI, g.GetSMIGPUPersistenceMode())
		}
	}

	if i.NVML != nil {
		for _, device := range i.NVML.DeviceInfos {
			o.PersistenceModesNVML = append(o.PersistenceModesNVML, device.PersistenceMode)
		}
	}

	return o
}

type Output struct {
	PersistencedExists  bool `json:"persistenced_exists"`
	PersistencedRunning bool `json:"persistenced_running"`

	PersistenceModesSMI  []nvidia_query.SMIGPUPersistenceMode `json:"persistence_modes_smi"`
	PersistenceModesNVML []nvidia_query_nvml.PersistenceMode  `json:"persistence_modes_nvml"`
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
	StateNamePersistenceMode = "persistence_mode"

	StateKeyPersistenceModeData       = "data"
	StateKeyPersistenceModeEncoding   = "encoding"
	StateValueMemoryUsageEncodingJSON = "json"
)

func ParseStatePersistenceMode(m map[string]string) (*Output, error) {
	data := m[StateKeyPersistenceModeData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNamePersistenceMode:
			o, err := ParseStatePersistenceMode(state.ExtraInfo)
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
	reasons := []string{}

	enabled := true
	for _, p := range o.PersistenceModesSMI {
		if o.PersistencedRunning {
			continue
		}

		// legacy mode (https://docs.nvidia.com/deploy/driver-persistence/index.html#installation)
		// "The reason why we cannot immediately deprecate the legacy persistence mode and switch transparently to the NVIDIA Persistence Daemon is because at this time,
		// we cannot guarantee that the NVIDIA Persistence Daemon will be running. This would be a feature regression as persistence mode might not be available out-of- the-box."
		if !p.Enabled {
			reasons = append(reasons, fmt.Sprintf("persistence mode is not enabled on %s (nvidia-smi)", p.ID))
			enabled = false
		}
	}

	for _, p := range o.PersistenceModesNVML {
		if o.PersistencedRunning {
			continue
		}

		// legacy mode (https://docs.nvidia.com/deploy/driver-persistence/index.html#installation)
		// "The reason why we cannot immediately deprecate the legacy persistence mode and switch transparently to the NVIDIA Persistence Daemon is because at this time,
		// we cannot guarantee that the NVIDIA Persistence Daemon will be running. This would be a feature regression as persistence mode might not be available out-of- the-box."
		if !p.Enabled {
			reasons = append(reasons, fmt.Sprintf("persistence mode is not enabled on %s (NVML)", p.UUID))
			enabled = false
		}
	}

	// does not make the component unhealthy, since persistence mode can still be enabled
	// recommend installing nvidia-persistenced since it's the recommended way to enable persistence mode
	if !o.PersistencedExists {
		reasons = append(reasons, "nvidia-persistenced does not exist (install 'nvidia-persistenced' or run 'nvidia-smi -pm 1')")
	}
	if !o.PersistencedRunning {
		reasons = append(reasons, "nvidia-persistenced exists but not running (start 'nvidia-persistenced' or run 'nvidia-smi -pm 1')")
	}

	return strings.Join(reasons, "; "), enabled, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNamePersistenceMode,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyPersistenceModeData:     string(b),
			StateKeyPersistenceModeEncoding: StateValueMemoryUsageEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
