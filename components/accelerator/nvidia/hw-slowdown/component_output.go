package hwslowdown

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/leptonai/gpud/components"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	o := &Output{}

	if i.NVML != nil {
		for _, devInfo := range i.NVML.DeviceInfos {
			if devInfo.ClockEvents != nil {
				o.HWSlowdownEventsNVML = append(o.HWSlowdownEventsNVML, *devInfo.ClockEvents)
			}
		}
	}

	return o
}

type Output struct {
	HWSlowdownEventsNVML []nvidia_query_nvml.ClockEvents `json:"hw_slowdown_events_nvml"`
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
	StateNameHWSlowdown = "hw_slowdown"

	StateKeyHWSlowdownData           = "data"
	StateKeyHWSlowdownEncoding       = "encoding"
	StateValueHWSlowdownEncodingJSON = "json"
)

func ParseStateHWSlowdown(m map[string]string) (*Output, error) {
	data := m[StateKeyHWSlowdownData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameHWSlowdown:
			o, err := ParseStateHWSlowdown(state.ExtraInfo)
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

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()

	reasons := make([]string, 0)
	for _, clockEvents := range o.HWSlowdownEventsNVML {
		if len(clockEvents.HWSlowdownReasons) > 0 {
			reasons = append(reasons, clockEvents.HWSlowdownReasons...)
		}
	}

	if len(reasons) == 0 {
		return []components.State{
			{
				Name:    StateNameHWSlowdown,
				Healthy: true,
				Reason:  "no hardware slowdown found",
				ExtraInfo: map[string]string{
					StateKeyHWSlowdownData:     string(b),
					StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
				},
			},
		}, nil
	}

	return []components.State{
		{
			Name:    StateNameHWSlowdown,
			Healthy: false,
			Reason:  "hw slowdown found: " + strings.Join(reasons, ", "),
			ExtraInfo: map[string]string{
				StateKeyHWSlowdownData:     string(b),
				StateKeyHWSlowdownEncoding: StateValueHWSlowdownEncodingJSON,
			},
		},
	}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(nvidia_component_error_xid_id.Name, cfg.Query, CreateGet())
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
				components_metrics.SetGetFailed(nvidia_component_error_xid_id.Name)
			} else {
				components_metrics.SetGetSuccess(nvidia_component_error_xid_id.Name)
			}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nvidia_query_nvml.DefaultInstanceReady():
		}

		// if there's no registered event, the channel blocks
		// then just return nil
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
