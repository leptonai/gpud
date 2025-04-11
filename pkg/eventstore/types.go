package eventstore

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

const (
	DefaultRetention = 3 * 24 * time.Hour // 3 days
)

type Store interface {
	Bucket(name string, opts ...OpOption) (Bucket, error)
}

type Bucket interface {
	Name() string
	Insert(ctx context.Context, ev apiv1.Event) error
	// Find returns nil if the event is not found.
	Find(ctx context.Context, ev apiv1.Event) (*apiv1.Event, error)
	// Get queries the event in the descending order of timestamp (latest event first).
	Get(ctx context.Context, since time.Time) ([]apiv1.Event, error)
	// Latest queries the latest event, returns nil if no event found.
	Latest(ctx context.Context) (*apiv1.Event, error)
	Purge(ctx context.Context, beforeTimestamp int64) (int, error)
	Close()
}

type Op struct {
	disablePurge bool
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
