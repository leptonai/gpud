package eventstore

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
)

const (
	DefaultRetention = 3 * 24 * time.Hour // 3 days
)

type Store interface {
	Bucket(name string, retention time.Duration) (Bucket, error)
}

type Bucket interface {
	Name() string
	Insert(ctx context.Context, ev components.Event) error
	Find(ctx context.Context, ev components.Event) (*components.Event, error)

	// Get queries the event in the descending order of timestamp (latest event first).
	Get(ctx context.Context, since time.Time) ([]components.Event, error)

	// Latest queries the latest event, returns nil if no event found.
	Latest(ctx context.Context) (*components.Event, error)

	Purge(ctx context.Context, beforeTimestamp int64) (int, error)
	Close()
}
