package gpm

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/query"
)

type Output struct {
	NVMLGPMEvent *nvidia_query_nvml.GPMEvent `json:"nvml_gpm_event,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

const (
	StateNameGPM = "gpm"

	StateKeyGPMData           = "data"
	StateKeyGPMEncoding       = "encoding"
	StateValueGPMEncodingJSON = "json"
)

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameGPM,
		Healthy: true,
		ExtraInfo: map[string]string{
			StateKeyGPMData:     string(b),
			StateKeyGPMEncoding: StateValueGPMEncodingJSON,
		},
	}
	return []components.State{state}, nil
}

func (o *Output) Events() []components.Event {
	return nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg nvidia_common.Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			Name,
			cfg.Query,
			CreateGet(),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

// DO NOT for-loop here
// the query.GetFunc is already called periodically in a loop by the poller
func CreateGet() query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nvidia_query_nvml.DefaultInstanceReady():
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case ev := <-nvidia_query_nvml.DefaultInstance().RecvGPMEvents():
			return ev, nil

		default:
			return nil, nil
		}
	}
}
