package store

import (
	"time"

	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

// Store defines the interface for storing IB ports states.
type Store interface {
	// Insert inserts the IB ports into the store.
	// The timestamp is the time when the IB ports were queried,
	// and all ports are inserted with the same timestamp.
	// Only stores the "Infiniband" link layer ports (not "Ethernet" or "Unknown").
	Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error

	// SetEventType sets the event id for the given timestamp, device, and port.
	SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error

	// Events returns the events since the given timestamp.
	// The events are sorted by timestamp in ascending order.
	Events(since time.Time) ([]Event, error)

	// Tombstone marks the given timestamp as tombstone,
	// where all the events before the given timestamp are discarded.
	// Useful when a component needs to discard (and purge) old events
	// after the admin action (e.g. reboot).
	Tombstone(timestamp time.Time) error

	// Scan scans the recent events to mark any events
	// (such as "ib port drop").
	Scan() error
}
