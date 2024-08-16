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

	"github.com/nxadm/tail"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type State struct {
	File            string           `json:"file"`
	LastSeekInfo    tail.SeekInfo    `json:"last_seek_info"`
	TailScanMatched []query_log.Item `json:"tail_scan_matched"`
}

func (s *State) JSON() ([]byte, error) {
	return json.Marshal(s)
}

func ParseStateJSON(data []byte) (*State, error) {
	s := new(State)
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

const (
	StateNameDmesg = "dmesg"

	StateKeyDmesgFile           = "file"
	StateKeyDmesgLastSeekOffset = "offset"
	StateKeyDmesgLastSeekWhence = "whence"

	StateNameDmesgTailScanMatched = "dmesg_tail_matched"

	StateKeyDmesgTailScanMatchedUnixSeconds = "unix_seconds"
	StateKeyDmesgTailScanMatchedLine        = "line"
	StateKeyDmesgTailScanMatchedFilter      = "filter"
	StateKeyDmesgTailScanMatchedError       = "error"
)

func ParseStateDmesg(s *State, m map[string]string) error {
	s.File = m[StateKeyDmesgFile]

	offset, err := strconv.ParseInt(m[StateKeyDmesgLastSeekOffset], 10, 64)
	if err != nil {
		return err
	}
	s.LastSeekInfo.Offset = offset

	whence, err := strconv.ParseInt(m[StateKeyDmesgLastSeekWhence], 10, 32)
	if err != nil {
		return err
	}
	s.LastSeekInfo.Whence = int(whence)

	return nil
}

func ParseStateDmesgTailScanMatched(m map[string]string) (query_log.Item, error) {
	ev := query_log.Item{}

	if m[StateKeyDmesgTailScanMatchedUnixSeconds] != "" {
		unixSeconds, err := strconv.ParseInt(m[StateKeyDmesgTailScanMatchedUnixSeconds], 10, 64)
		if err != nil {
			return query_log.Item{}, err
		}
		ev.Time = metav1.Time{Time: time.Unix(unixSeconds, 0)}
	}
	ev.Line = m[StateKeyDmesgTailScanMatchedLine]

	var f *query_log_filter.Filter
	if m[StateKeyDmesgTailScanMatchedFilter] != "" {
		var err error
		f, err = query_log_filter.ParseFilterJSON([]byte(m[StateKeyDmesgTailScanMatchedFilter]))
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

func ParseStates(states ...components.State) (*State, error) {
	s := &State{}
	for _, state := range states {
		switch state.Name {
		case StateNameDmesg:
			if err := ParseStateDmesg(s, state.ExtraInfo); err != nil {
				return nil, err
			}

		case StateNameDmesgTailScanMatched:
			ev, err := ParseStateDmesgTailScanMatched(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			s.TailScanMatched = append(s.TailScanMatched, ev)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return s, nil
}

func (s *State) States() []components.State {
	cs := make([]components.State, 0)
	cs = append(cs, components.State{
		Name:    StateNameDmesg,
		Healthy: true,
		Reason:  fmt.Sprintf("scanning file: %s", s.File),
		ExtraInfo: map[string]string{
			StateKeyDmesgFile:           s.File,
			StateKeyDmesgLastSeekOffset: fmt.Sprintf("%d", s.LastSeekInfo.Offset),
			StateKeyDmesgLastSeekWhence: fmt.Sprintf("%d", s.LastSeekInfo.Whence),
		},
	})

	if len(s.TailScanMatched) > 0 {
		for _, item := range s.TailScanMatched {
			b, _ := item.Matched.JSON()
			es := ""
			if item.Error != nil {
				es = item.Error.Error()
			}
			cs = append(cs, components.State{
				Name:    StateNameDmesgTailScanMatched,
				Healthy: item.Error == nil,
				Reason:  fmt.Sprintf("matched line: %s (filter %s)", item.Line, string(b)),
				ExtraInfo: map[string]string{
					StateKeyDmesgTailScanMatchedUnixSeconds: fmt.Sprintf("%d", item.Time.Unix()),
					StateKeyDmesgTailScanMatchedLine:        item.Line,
					StateKeyDmesgTailScanMatchedFilter:      string(b),
					StateKeyDmesgTailScanMatchedError:       es,
				},
			})
		}
	} else {
		cs = append(cs, components.State{
			Name:    StateNameDmesgTailScanMatched,
			Healthy: true,
			Reason:  "no matched line",
		})
	}
	return cs
}
