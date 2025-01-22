package xid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/common"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
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
		return Reason{}
	}

	reason := Reason{}

	if o.NVMLXidEvent != nil {
		var suggestedActions *common.SuggestedActions = nil
		if o.NVMLXidEvent.Detail != nil && o.NVMLXidEvent.Detail.SuggestedActionsByGPUd != nil {
			suggestedActions = o.NVMLXidEvent.Detail.SuggestedActionsByGPUd
		}

		xidErr := XidError{
			Time: o.NVMLXidEvent.Time,

			DataSource: "nvml",

			DeviceUUID: o.NVMLXidEvent.DeviceUUID,

			Xid: o.NVMLXidEvent.Xid,

			SuggestedActionsByGPUd:    suggestedActions,
			CriticalErrorMarkedByGPUd: o.NVMLXidEvent.Detail != nil && o.NVMLXidEvent.Detail.CriticalErrorMarkedByGPUd,
		}

		reason.Errors = append(reason.Errors, xidErr)
	}

	for _, de := range o.DmesgErrors {
		if de.Detail == nil {
			continue
		}

		xid := uint64(de.Detail.Xid)
		xidErr := XidError{
			Time: de.LogItem.Time,

			DataSource: "dmesg",

			DeviceUUID: de.DeviceUUID,

			Xid: xid,
		}
		if de.Detail != nil {
			xidErr.SuggestedActionsByGPUd = de.Detail.SuggestedActionsByGPUd
			xidErr.CriticalErrorMarkedByGPUd = de.Detail.CriticalErrorMarkedByGPUd
		}

		reason.Errors = append(reason.Errors, xidErr)
	}

	sort.Slice(reason.Errors, func(i, j int) bool {
		// puts earlier times first, latest time last
		return reason.Errors[i].Time.Before(&reason.Errors[j].Time)
	})
	for _, e := range reason.Errors {
		reason.Messages = append(reason.Messages,
			fmt.Sprintf("xid %d detected by %s (%s)",
				e.Xid, e.DataSource, humanize.Time(e.Time.UTC()),
			),
		)
	}
	return reason
}

const (
	EventNameErroXid = "error_xid"

	EventKeyErroXidUnixSeconds    = "unix_seconds"
	EventKeyErroXidData           = "data"
	EventKeyErroXidEncoding       = "encoding"
	EventValueErroXidEncodingJSON = "json"
)

func (o *Output) getEvents(since time.Time) []components.Event {
	reason := o.GetReason()

	des := make([]components.Event, 0)
	for i, xidErr := range reason.Errors {
		if xidErr.Time.IsZero() {
			log.Logger.Debugw("skipping event because it's too old", "xid", xidErr.Xid, "since", since, "event_time", xidErr.Time.Time)
			continue
		}
		if xidErr.Time.Time.Before(since) {
			log.Logger.Debugw("skipping event because it's too old", "xid", xidErr.Xid, "since", since, "event_time", xidErr.Time.Time)
			continue
		}

		msg := reason.Messages[i]
		xidErrBytes, _ := xidErr.JSON()

		des = append(des, components.Event{
			Time:    xidErr.Time,
			Name:    EventNameErroXid,
			Type:    common.EventTypeCritical,
			Message: msg,
			ExtraInfo: map[string]string{
				EventKeyErroXidUnixSeconds: strconv.FormatInt(xidErr.Time.Unix(), 10),
				EventKeyErroXidData:        string(xidErrBytes),
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
func setDefaultPoller(cfg nvidia_common.Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			nvidia_component_error_xid_id.Name,
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

		case ev := <-nvidia_query_nvml.DefaultInstance().RecvXidEvents():
			return ev, nil

		default:
			return nil, nil
		}
	}
}
