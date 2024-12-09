package sxid

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	"sigs.k8s.io/yaml"
)

type Output struct {
	DmesgErrors []nvidia_query_sxid.DmesgError `json:"dmesg_errors,omitempty"`
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

func (o *Output) GetReason() Reason {
	if len(o.DmesgErrors) == 0 {
		return Reason{}
	}

	reason := Reason{}

	for _, de := range o.DmesgErrors {
		if de.Detail == nil {
			continue
		}

		sxid := uint64(de.Detail.SXid)

		reason.Errors = append(reason.Errors, SXidError{
			Time: de.LogItem.Time,

			DataSource: "dmesg",

			DeviceUUID: "",

			SXid: sxid,

			SuggestedActionsByGPUd:    de.Detail.SuggestedActionsByGPUd,
			CriticalErrorMarkedByGPUd: de.Detail.CriticalErrorMarkedByGPUd,
		})
	}

	sort.Slice(reason.Errors, func(i, j int) bool {
		// puts earlier times first, latest time last
		return reason.Errors[i].Time.Before(&reason.Errors[j].Time)
	})
	for _, e := range reason.Errors {
		reason.Messages = append(reason.Messages,
			fmt.Sprintf("sxid %d detected by %s (%s)",
				e.SXid, e.DataSource, humanize.Time(e.Time.UTC()),
			),
		)
	}
	return reason
}

const (
	EventNameErroSXid = "error_sxid"

	EventKeyErroSXidUnixSeconds    = "unix_seconds"
	EventKeyErroSXidData           = "data"
	EventKeyErroSXidEncoding       = "encoding"
	EventValueErroSXidEncodingJSON = "json"
)

func (o *Output) getEvents(since time.Time) []components.Event {
	reason := o.GetReason()

	des := make([]components.Event, 0)
	for i, sxidErr := range reason.Errors {
		if sxidErr.Time.IsZero() {
			log.Logger.Warnw("skipping event because it's too old", "sxid", sxidErr.SXid, "since", since, "event_time", sxidErr.Time.Time)
			continue
		}
		if sxidErr.Time.Time.Before(since) {
			log.Logger.Warnw("skipping event because it's too old", "sxid", sxidErr.SXid, "since", since, "event_time", sxidErr.Time.Time)
			continue
		}

		msg := reason.Messages[i]
		sxidErrBytes, _ := sxidErr.JSON()

		des = append(des, components.Event{
			Time:    sxidErr.Time,
			Name:    EventNameErroSXid,
			Type:    "Critical",
			Message: msg,
			ExtraInfo: map[string]string{
				EventKeyErroSXidUnixSeconds: strconv.FormatInt(sxidErr.Time.Unix(), 10),
				EventKeyErroSXidData:        string(sxidErrBytes),
				EventKeyErroSXidEncoding:    StateValueErrorSXidEncodingJSON,
			},
		})
	}
	if len(des) == 0 {
		return nil
	}
	return des
}
