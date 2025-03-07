package badenvs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{
		FoundBadEnvsForCUDA: i.BadEnvVarsForCUDA,
	}
	return o
}

type Output struct {
	// FoundBadEnvsForCUDA is a map of environment variables that are known to hurt CUDA.
	// that is set globally for the host.
	// This implements "DCGM_FR_BAD_CUDA_ENV" logic in DCGM.
	FoundBadEnvsForCUDA map[string]string `json:"found_bad_envs_for_cuda"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameBadEnvs = "bad_envs"

	StateKeyUtilizationData           = "data"
	StateKeyUtilizationEncoding       = "encoding"
	StateValueUtilizationEncodingJSON = "json"
)

func (o *Output) States() ([]components.State, error) {
	reasons := []string{}
	for k, v := range o.FoundBadEnvsForCUDA {
		reasons = append(reasons, fmt.Sprintf("'%s' is set: %s", k, v))
	}
	reason := ""
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameBadEnvs,
		Healthy: len(o.FoundBadEnvsForCUDA) == 0,
		Reason:  reason,
		ExtraInfo: map[string]string{
			StateKeyUtilizationData:     string(b),
			StateKeyUtilizationEncoding: StateValueUtilizationEncodingJSON,
		},
	}
	return []components.State{state}, nil
}
