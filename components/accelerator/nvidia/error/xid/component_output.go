package xid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/leptonai/gpud/components"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/common"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	"sigs.k8s.io/yaml"
)

type Output struct {
	DmesgErrors  []nvidia_query_xid.DmesgError `json:"dmesg_errors,omitempty"`
	NVMLXidEvent *nvidia_query_nvml.XidEvent   `json:"nvml_xid_event,omitempty"`
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

type NVMLError struct {
	Xid   uint64 `json:"xid"`
	Error error  `json:"error"`
}

func (nv *NVMLError) JSON() ([]byte, error) {
	return json.Marshal(nv)
}

func ParseNVMLErrorJSON(data []byte) (*NVMLError, error) {
	nv := new(NVMLError)
	if err := json.Unmarshal(data, nv); err != nil {
		return nil, err
	}
	return nv, nil
}

func (nv *NVMLError) YAML() ([]byte, error) {
	return yaml.Marshal(nv)
}

func ParseNVMLErrorYAML(data []byte) (*NVMLError, error) {
	nv := new(NVMLError)
	if err := yaml.Unmarshal(data, nv); err != nil {
		return nil, err
	}
	return nv, nil
}

const (
	StateNameErrorXid = "error_xid"

	StateKeyErrorXidData           = "data"
	StateKeyErrorXidEncoding       = "encoding"
	StateValueErrorXidEncodingJSON = "json"
)

func ParseStateErrorXid(m map[string]string) (*Output, error) {
	data := m[StateKeyErrorXidData]
	return ParseOutputJSON([]byte(data))
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameErrorXid:
			o, err := ParseStateErrorXid(state.ExtraInfo)
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

func (o *Output) GetReason() Reason {
	if len(o.DmesgErrors) == 0 && (o.NVMLXidEvent == nil || o.NVMLXidEvent.Detail == nil) {
		return Reason{
			Messages: []string{"no xid error found"},
		}
	}

	// non-zero xid events, thus state itself as unhealthy
	reason := Reason{
		Errors: make(map[uint64]XidError),
	}

	if o.NVMLXidEvent != nil {
		var suggestedActions *common.SuggestedActions = nil
		if o.NVMLXidEvent.Detail != nil && o.NVMLXidEvent.Detail.SuggestedActionsByGPUd != nil {
			suggestedActions = o.NVMLXidEvent.Detail.SuggestedActionsByGPUd
		}

		reason.Errors[o.NVMLXidEvent.Xid] = XidError{
			DataSource: "nvml",

			DeviceUUID: o.NVMLXidEvent.DeviceUUID,

			Xid: o.NVMLXidEvent.Xid,

			SuggestedActionsByGPUd:    suggestedActions,
			CriticalErrorMarkedByGPUd: o.NVMLXidEvent.Detail != nil && o.NVMLXidEvent.Detail.CriticalErrorMarkedByGPUd,
		}
	}

	if len(o.DmesgErrors) > 0 {
		for _, de := range o.DmesgErrors {
			if de.Detail == nil {
				continue
			}

			xid := uint64(de.Detail.Xid)

			// already detected by NVML wait/watch API
			if _, ok := reason.Errors[xid]; ok {
				continue
			}

			reason.Errors[xid] = XidError{
				DataSource: "dmesg",

				DeviceUUID: "",

				Xid: xid,

				SuggestedActionsByGPUd:    de.Detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: de.Detail.CriticalErrorMarkedByGPUd,
			}
		}
	}

	return reason
}

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate(onlyGPUdCritical bool) (Reason, bool, error) {
	reason := o.GetReason()

	// if none of the Xid errors is marked as critical in GPUd,
	// the component is then healthy
	// we still provide other information where the monitoring component
	// can still take its own action
	healthy := true
	criticals := make(map[uint64]XidError)
	for _, e := range reason.Errors {
		if e.CriticalErrorMarkedByGPUd {
			healthy = false
			criticals[e.Xid] = e
		}
	}
	if onlyGPUdCritical {
		reason.Errors = criticals
	}

	return reason, healthy, nil
}

func (o *Output) States() ([]components.State, error) {
	reason, healthy, err := o.Evaluate(true)
	if err != nil {
		return nil, err
	}

	reasonB, err := reason.JSON()
	if err != nil {
		return nil, err
	}

	b, err := o.JSON()
	if err != nil {
		return nil, err
	}

	state := components.State{
		Name:    StateNameErrorXid,
		Healthy: healthy,
		Reason:  string(reasonB),
		ExtraInfo: map[string]string{
			StateKeyErrorXidData:     string(b),
			StateKeyErrorXidEncoding: StateValueErrorXidEncodingJSON,
		},
	}

	return []components.State{state}, nil
}

const (
	EventNameErroXid = "error_xid"

	EventKeyErroXidUnixSeconds    = "unix_seconds"
	EventKeyErroXidData           = "data"
	EventKeyErroXidEncoding       = "encoding"
	EventValueErroXidEncodingJSON = "json"
)

func (o *Output) Events() []components.Event {
	des := make([]components.Event, 0)
	for _, de := range o.DmesgErrors {
		b, _ := de.JSON()

		des = append(des, components.Event{
			Time: de.LogItem.Time,
			Name: EventNameErroXid,
			ExtraInfo: map[string]string{
				EventKeyErroXidUnixSeconds: strconv.FormatInt(de.LogItem.Time.Unix(), 10),
				EventKeyErroXidData:        string(b),
				EventKeyErroXidEncoding:    StateValueErrorXidEncodingJSON,
			},
		})
	}
	if len(des) == 0 {
		return nil
	}
	return des
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet())
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

		// if there's no registered event, the channel blocks
		// then just return nil
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case ev := <-nvidia_query_nvml.DefaultInstance().RecvXidEvents():
			return ev, nil

		default:
			return nil, nil
		}
	}
}
