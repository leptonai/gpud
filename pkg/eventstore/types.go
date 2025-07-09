package eventstore

import (
	"context"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

var (
	ErrEventNamesToSelectAndExclude = errors.New("cannot use both event names to select/exclude")
)

type Events []Event

func (evs Events) Events() apiv1.Events {
	events := make(apiv1.Events, len(evs))
	for i, ev := range evs {
		events[i] = ev.ToEvent()
	}
	return events
}

// Event represents an entry in the event store.
type Event struct {
	// Component represents which component generated the event.
	Component string

	// Time represents when the event happened
	Time time.Time

	// Name represents the name of the event.
	Name string

	// Type represents the type of the event.
	Type string

	// Message represents the detailed message of the event.
	Message string

	// ExtraInfo represents the extra information of the event.
	ExtraInfo map[string]string
}

func (e *Event) ToEvent() apiv1.Event {
	return apiv1.Event{
		Component: e.Component,
		Time:      metav1.Time{Time: e.Time},
		Name:      e.Name,
		Type:      apiv1.EventType(e.Type),
		Message:   e.Message,
	}
}

const DefaultRetention = 3 * 24 * time.Hour // 3 days

type Store interface {
	Bucket(name string, opts ...OpOption) (Bucket, error)
}

type Bucket interface {
	Name() string
	Insert(ctx context.Context, ev Event) error
	// Find returns nil if the event is not found.
	Find(ctx context.Context, ev Event) (*Event, error)
	// Get queries the event in the descending order of timestamp (latest event first).
	Get(ctx context.Context, since time.Time, opts ...OpOption) (Events, error)
	// Latest queries the latest event, returns nil if no event found.
	Latest(ctx context.Context) (*Event, error)
	Purge(ctx context.Context, beforeTimestamp int64) (int, error)
	Close()
}

type Op struct {
	disablePurge        bool
	eventNamesToSelect  map[string]any
	eventNamesToExclude map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if len(op.eventNamesToSelect) > 0 && len(op.eventNamesToExclude) > 0 {
		return ErrEventNamesToSelectAndExclude
	}

	return nil
}

// WithDisablePurge specifies that the purge should be disabled.
// This is useful for loading the bucket for read-only operations.
func WithDisablePurge() OpOption {
	return func(op *Op) {
		op.disablePurge = true
	}
}

// WithEventNamesToSelect specifies the event names to select from the query.
// This is useful when a bucket/table contains multiple event names,
// and we want to select only a subset of them.
// e.g., select only "reboot" events from the "os" bucket.
func WithEventNamesToSelect(eventNames ...string) OpOption {
	return func(op *Op) {
		if op.eventNamesToSelect == nil {
			op.eventNamesToSelect = make(map[string]any)
		}
		for _, eventName := range eventNames {
			op.eventNamesToSelect[eventName] = true
		}
	}
}

// WithEventNamesToExclude specifies the event names to exclude from the query.
// This is useful when a bucket/table contains multiple event names,
// and we want to exclude a subset of them.
// e.g., exclude "reboot" events from the "os" bucket.
func WithEventNamesToExclude(eventNames ...string) OpOption {
	return func(op *Op) {
		if op.eventNamesToExclude == nil {
			op.eventNamesToExclude = make(map[string]any)
		}
		for _, eventName := range eventNames {
			op.eventNamesToExclude[eventName] = true
		}
	}
}
