package xidsxidstate

import (
	"errors"
	"time"
)

var ErrInvalidLimit = errors.New("limit must be greater than or equal to 0")

type Op struct {
	eventType               string
	sinceUnixSeconds        int64
	beforeUnixSeconds       int64
	sortUnixSecondsAscOrder bool
	limit                   int
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

func WithEventType(eventType string) OpOption {
	return func(op *Op) {
		op.eventType = eventType
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

// WithSortUnixSecondsAscendingOrder sorts the events by unix_seconds in ascending order,
// meaning its read query returns the oldest events first.
func WithSortUnixSecondsAscendingOrder() OpOption {
	return func(op *Op) {
		op.sortUnixSecondsAscOrder = true
	}
}

// WithSortUnixSecondsDescendingOrder sorts the events by unix_seconds in descending order,
// meaning its read query returns the newest events first.
func WithSortUnixSecondsDescendingOrder() OpOption {
	return func(op *Op) {
		op.sortUnixSecondsAscOrder = false
	}
}

func WithLimit(limit int) OpOption {
	return func(op *Op) {
		op.limit = limit
	}
}
