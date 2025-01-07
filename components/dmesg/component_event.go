package dmesg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/components"
	query_log "github.com/leptonai/gpud/components/query/log"
	"github.com/leptonai/gpud/log"
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

	EventKeyDmesgMatchedError   = "error"
	EventKeyDmesgMatchedLogItem = "log_item"
)

// TODO: deprecate
func (ev *Event) Events() []components.Event {
	if len(ev.Matched) == 0 {
		return nil
	}

	evs := make([]components.Event, 0)
	for _, logItem := range ev.Matched {
		msg := logItem.Line
		if logItem.Matched != nil && len(logItem.Matched.OwnerReferences) > 0 {
			msg = fmt.Sprintf("%s (%s)", logItem.Line, strings.Join(logItem.Matched.OwnerReferences, ","))
		}

		es := ""
		if logItem.Error != nil {
			es = *logItem.Error
		}

		ob, err := logItem.JSON()
		if err != nil {
			log.Logger.Errorw("failed to marshal log item", "error", err)
			continue
		}

		evs = append(evs, components.Event{
			Time: logItem.Time,
			Name: EventNameDmesgMatched,

			// criticality should be decided in individual components
			Type: components.EventTypeWarning,

			Message: msg,
			ExtraInfo: map[string]string{
				EventKeyDmesgMatchedError:   es,
				EventKeyDmesgMatchedLogItem: string(ob),
			},
		})
	}
	if len(evs) == 0 {
		return nil
	}
	return evs
}
