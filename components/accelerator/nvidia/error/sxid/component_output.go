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
		return Reason{
			Messages: []string{"no sxid error found"},
		}
	}

	reason := Reason{
		Messages: make([]string, 0),
		Errors:   make(map[uint64]SXidError),
	}

	for _, de := range o.DmesgErrors {
		if de.Detail == nil {
			continue
		}

		sxid := uint64(de.Detail.SXid)

		// already detected by previous dmesg event
		// only overwrite if it's more recent
		// in other words, we skip if this is the older dmesg event
		prev, ok := reason.Errors[sxid]
		if ok && prev.Time.After(de.LogItem.Time.UTC()) {
			continue
		}

		// either never found by previous dmesg event or found newer dmesg event
		// thus insert or overwrite
		reason.Errors[sxid] = SXidError{
			Time: de.LogItem.Time,

			DataSource: "dmesg",

			DeviceUUID: "",

			SXid: sxid,

			SuggestedActionsByGPUd:    de.Detail.SuggestedActionsByGPUd,
			CriticalErrorMarkedByGPUd: de.Detail.CriticalErrorMarkedByGPUd,
		}

		reason.Messages = append(reason.Messages,
			fmt.Sprintf(
				"sxid %d detected by dmesg (%s)",
				sxid,
				humanize.Time(de.LogItem.Time.UTC()),
			),
		)
	}

	return reason
}

func (o *Output) getStates() ([]components.State, error) {
	outputBytes, err := o.JSON()
	if err != nil {
		return nil, err
	}

	reason := o.GetReason()

	// to overwrite the reason with only critical errors
	criticals := make(map[uint64]SXidError)
	for _, e := range reason.Errors {
		if e.CriticalErrorMarkedByGPUd {
			criticals[e.SXid] = e
		}
	}
	reason.Errors = criticals

	reasonBytes, err := reason.JSON()
	if err != nil {
		return nil, err
	}

	state := components.State{
		Name: StateNameErrorSXid,

		// only unhealthy if critical sxid is found
		// see events for non-critical sxids
		Healthy: len(reason.Errors) > 0,
		Reason:  string(reasonBytes),

		ExtraInfo: map[string]string{
			StateKeyErrorSXidData:     string(outputBytes),
			StateKeyErrorSXidEncoding: StateValueErrorSXidEncodingJSON,
		},
	}

	return []components.State{state}, nil
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

	nonCriticals := make(map[uint64]SXidError)
	for _, e := range reason.Errors {
		if e.CriticalErrorMarkedByGPUd {
			log.Logger.Warnw("skipping sxid event for /events due to being critical", "sxid", e.SXid, "time", e.Time, "since", since)
			continue
		}

		// if the event is older than since or undefined, skip
		if e.Time.IsZero() || e.Time.Time.Before(since) {
			log.Logger.Warnw("skipping sxid event for /events due to being undefined time or too old", "sxid", e.SXid, "time", e.Time, "since", since)
			continue
		}

		nonCriticals[e.SXid] = e
	}

	des := make([]components.Event, 0)
	for _, sxidErr := range nonCriticals {
		sxidErrBytes, _ := sxidErr.JSON()

		des = append(des, components.Event{
			Time: sxidErr.Time,
			Name: EventNameErroSXid,
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

	sort.Slice(des, func(i, j int) bool {
		// puts earlier times first, latest time last
		return des[i].Time.Before(&des[j].Time)
	})
	return des
}
