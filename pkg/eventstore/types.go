package eventstore

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
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

// TODO: deprecate this in favor of StoreV2
type Store interface {
	Bucket(name string, opts ...OpOption) (Bucket, error)
}

type StoreV2 interface {
	BucketV2(name string, opts ...OpOption) (BucketV2, error)
}

// TODO: deprecate this in favor of BucketV2
type Bucket interface {
	// Name returns the name of the bucket.
	Name() string

	// Insert inserts an event into the bucket.
	Insert(ctx context.Context, ev Event) error

	// Find returns nil if the event is not found.
	Find(ctx context.Context, ev Event) (*Event, error)

	// Get queries the event in the descending order of timestamp (latest event first).
	// TODO: deprecate this in favor of Read
	Get(ctx context.Context, since time.Time) (Events, error)

	// Latest queries the latest event, returns nil if no event found.
	Latest(ctx context.Context) (*Event, error)

	// Purge purges the events older than the given timestamp.
	// Returns the number of purged events.
	Purge(ctx context.Context, beforeTimestamp int64) (int, error)

	// Close closes the bucket.
	Close()
}

// BucketV2 is the same as Bucket, but with the Read method.
type BucketV2 interface {
	Bucket

	// Read queries the events in the descending order of timestamp (latest event first).
	// If the "since" is not provided, it returns all events.
	Read(ctx context.Context, opts ...OpOption) (Events, error)
}

type Op struct {
	disablePurge bool
	since        time.Time
	name         string
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
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

func WithSince(since time.Time) OpOption {
	return func(op *Op) {
		op.since = since
	}
}

func WithName(name string) OpOption {
	return func(op *Op) {
		op.name = name
	}
}
