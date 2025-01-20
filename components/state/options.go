package state

import (
	"errors"
	"time"
)

var ErrInvalidLimit = errors.New("limit must be greater than or equal to 0")

type Op struct {
	dataSource            string
	eventType             string
	target                string
	sinceUnixSeconds      int64
	beforeUnixSeconds     int64
	sortTimestampAscOrder bool
	limit                 int
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.limit < 0 {
		return ErrInvalidLimit
	}

	return nil
}

func WithDataSource(dataSource string) OpOption {
	return func(op *Op) {
		op.dataSource = dataSource
	}
}

// WithEventType sets the event type for the select queries.
func WithEventType(eventType string) OpOption {
	return func(op *Op) {
		op.eventType = eventType
	}
}

// WithTarget sets the target for the select queries.
func WithTarget(target string) OpOption {
	return func(op *Op) {
		op.target = target
	}
}

// WithSince sets the since timestamp for the select queries.
// If not specified, it returns all events.
func WithSince(t time.Time) OpOption {
	return func(op *Op) {
		op.sinceUnixSeconds = t.UTC().Unix()
	}
}

// WithBefore sets the before timestamp for the delete queries.
// If not specified, it deletes all events.
func WithBefore(t time.Time) OpOption {
	return func(op *Op) {
		op.beforeUnixSeconds = t.UTC().Unix()
	}
}

// WithSortTimestampAscendingOrder sorts the events by unix_seconds in ascending order,
// meaning its read query returns the oldest events first.
func WithSortTimestampAscendingOrder() OpOption {
	return func(op *Op) {
		op.sortTimestampAscOrder = true
	}
}

// WithSortTimestampDescendingOrder sorts the events by unix_seconds in descending order,
// meaning its read query returns the newest events first.
func WithSortTimestampDescendingOrder() OpOption {
	return func(op *Op) {
		op.sortTimestampAscOrder = false
	}
}

func WithLimit(limit int) OpOption {
	return func(op *Op) {
		op.limit = limit
	}
}
