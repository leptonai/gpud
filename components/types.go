package components

import (
	"context"
	"fmt"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Component represents an individual component of the system.
//
// Each component check is independent of each other.
// But the underlying implementation may share the same data sources
// in order to minimize the querying overhead (e.g., nvidia-smi calls).
//
// Each component implements its own output format inside the State struct.
// And recommended to have a consistent name for its HTTP handler.
// And recommended to define const keys for the State extra information field.
type Component interface {
	// Defines the component name,
	// and used for the HTTP handler registration path.
	// Must be globally unique.
	Name() string

	// Start called upon server start.
	// Implements component-specific poller start logic.
	Start() error

	// HealthStates returns the current health states of the component.
	HealthStates(ctx context.Context) (apiv1.HealthStates, error)

	// Events returns all the events from "since".
	Events(ctx context.Context, since time.Time) (apiv1.Events, error)

	// Called upon server close.
	// Implements copmonent-specific poller cleanup logic.
	Close() error
}

// HealthSettable is an optional interface that can be implemented by components
// to allow setting the health state.
type HealthSettable interface {
	// SetHealthy sets the health state to healthy.
	SetHealthy() error
}

// HealthStateCheckResult is the data type that represents the result of
// a component health state check.
type HealthStateCheckResult interface {
	// String returns a string representation of the data.
	// Describes the data in a human-readable format.
	fmt.Stringer

	// Summary returns a summary of the check result.
	Summary() string

	// HealthState returns the health state of the check result.
	HealthState() apiv1.HealthStateType
}
