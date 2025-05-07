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

	// Tags returns a list of tags that describe the component.
	//
	// The component tags are static, and will not change over time.
	// It is important to keep in mind that, the tags only represent
	// the component's functionality, but not the component's health state.
	//
	// This is useful to trigger on-demand component checks based on specific tags.
	//
	// e.g.,
	// GPU enabled components may return its accelerator manufacturer,
	// but does not report its health state via the tags.
	Tags() []string

	// IsSupported returns true if the component is supported on the current machine.
	// For example, this returns "false" if a component requires NVIDIA GPUs,
	// but the machine does not have NVIDIA GPUs.
	IsSupported() bool

	// Start called upon server start.
	// Implements component-specific poller start logic.
	Start() error

	// Check triggers the component check once, and returns the latest health check result.
	// This is used for one-time checks, such as "gpud scan".
	// It is up to the component to decide the check timeouts.
	// Thus, we do not pass the context here.
	// The CheckResult should embed the timeout errors if any, via Summary and HealthState.
	Check() CheckResult

	// LastHealthStates reads the latest health states of the component,
	// cached from its periodic checks.
	// If no check has been performed, it returns a single health state of apiv1.StateTypeHealthy.
	LastHealthStates() apiv1.HealthStates

	// Events returns all the events from "since".
	Events(ctx context.Context, since time.Time) (apiv1.Events, error)

	// Called upon server close.
	// Implements copmonent-specific poller cleanup logic.
	Close() error
}

// Deregisterable is an interface that allows a custom plugin to be deregistered.
// By default, the regular/built-in components are not allowed to be deregistered,
// unless it implements this interface.
type Deregisterable interface {
	// CanDeregister returns true if the custom plugin can be deregistered.
	CanDeregister() bool
}

// HealthSettable is an optional interface that can be implemented by components
// to allow setting the health state.
type HealthSettable interface {
	// SetHealthy sets the health state to healthy.
	SetHealthy() error
}

// CheckResult is the data type that represents the result of
// a component health state check.
type CheckResult interface {
	// ComponentName returns the name of the component that produced this check result.
	ComponentName() string

	// String returns a string representation of the data.
	// Describes the data in a human-readable format.
	fmt.Stringer

	// Summary returns a summary of the check result.
	Summary() string

	// HealthStateType returns the health state of the last check result.
	HealthStateType() apiv1.HealthStateType
	// HealthStates returns the health states of the last check result.
	HealthStates() apiv1.HealthStates
}

// CheckResultDebugger is an optional interface that can be implemented by components
// to allow debugging the check result.
type CheckResultDebugger interface {
	// Debug returns a string representation of the check result.
	Debug() string
}
