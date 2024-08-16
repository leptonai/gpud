package dmesg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Event struct {
	Matched []query_log.Item `json:"matched"`
}

func (ev *Event) JSON() ([]byte, error) {
	return json.Marshal(ev)
}

func ParseEventJSON(data []byte) (*Event, error) {
	ev := new(Event)
	if err := json.Unmarshal(data, ev); err != nil {
		return nil, err
	}
	return ev, nil
}

const (
	EventNameDmesgMatched = "dmesg_matched"

	EventKeyDmesgMatchedUnixSeconds = "unix_seconds"
	EventKeyDmesgMatchedLine        = "line"
	EventKeyDmesgMatchedFilter      = "filter"
	EventKeyDmesgMatchedError       = "error"
)

func ParseEventDmesgMatched(m map[string]string) (query_log.Item, error) {
	ev := query_log.Item{}

	unixSeconds, err := strconv.ParseInt(m[EventKeyDmesgMatchedUnixSeconds], 10, 64)
	if err != nil {
		return query_log.Item{}, err
	}
	ev.Time = metav1.Time{Time: time.Unix(unixSeconds, 0)}
	ev.Line = m[EventKeyDmesgMatchedLine]

	var f *query_log_filter.Filter
	if m[EventKeyDmesgMatchedFilter] != "" {
		f, err = query_log_filter.ParseFilterJSON([]byte(m[EventKeyDmesgMatchedFilter]))
		if err != nil {
			return query_log.Item{}, err
		}
	} else {
		f = nil
	}
	ev.Matched = f

	if m[EventKeyDmesgMatchedError] != "" {
		ev.Error = errors.New(m[EventKeyDmesgMatchedError])
	}

	return ev, nil
}

func ParseEvents(events ...components.Event) (*Event, error) {
	ev := &Event{}
	for _, e := range events {
		switch e.Name {
		case EventNameDmesgMatched:
			item, err := ParseEventDmesgMatched(e.ExtraInfo)
			if err != nil {
				return nil, err
			}
			ev.Matched = append(ev.Matched, item)

		default:
			return nil, fmt.Errorf("unknown event name: %s", e.Name)
		}
	}
	return ev, nil
}

func (ev *Event) Events() []components.Event {
	if len(ev.Matched) == 0 {
		return nil
	}
	evs := make([]components.Event, 0)
	for _, ev := range ev.Matched {
		b, _ := ev.Matched.JSON()
		es := ""
		if ev.Error != nil {
			es = ev.Error.Error()
		}
		evs = append(evs, components.Event{
			Time: ev.Time,
			Name: EventNameDmesgMatched,
			ExtraInfo: map[string]string{
				EventKeyDmesgMatchedUnixSeconds: fmt.Sprintf("%d", ev.Time.Unix()),
				EventKeyDmesgMatchedLine:        ev.Line,
				EventKeyDmesgMatchedFilter:      string(b),
				EventKeyDmesgMatchedError:       es,
			},
		})
	}
	if len(evs) == 0 {
		return nil
	}
	return evs
}
