// Package components defines the common interfaces for the components.
package components

import (
	"context"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Returns the current states of the component.
	States(ctx context.Context) ([]State, error)

	// Returns all the events from "since".
	Events(ctx context.Context, since time.Time) ([]Event, error)

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

type State struct {
	Name      string            `json:"name,omitempty"`
	Healthy   bool              `json:"healthy,omitempty"`
	Health    string            `json:"health,omitempty"`     // Healthy, Degraded, Unhealthy
	Reason    string            `json:"reason,omitempty"`     // a detailed and processed reason on why the component is not healthy
	Error     string            `json:"error,omitempty"`      // the unprocessed error returned from the component
	ExtraInfo map[string]string `json:"extra_info,omitempty"` // any extra information the component may want to expose

	SuggestedActions *SuggestedActions `json:"suggested_actions,omitempty"`
}

const (
	StateHealthy      = "Healthy"
	StateUnhealthy    = "Unhealthy"
	StateInitializing = "Initializing"
	StateDegraded     = "Degraded"
)

type Event struct {
	Time             metav1.Time       `json:"time"`
	Name             string            `json:"name,omitempty"`
	Type             EventType         `json:"type,omitempty"`
	Message          string            `json:"message,omitempty"`    // detailed message of the event
	ExtraInfo        map[string]string `json:"extra_info,omitempty"` // any extra information the component may want to expose
	SuggestedActions *SuggestedActions `json:"suggested_actions,omitempty"`
}

type Metric struct {
	UnixSeconds         int64   `json:"unix_seconds"`
	MetricName          string  `json:"metric_name"`
	MetricSecondaryName string  `json:"metric_secondary_name,omitempty"`
	Value               float64 `json:"value"`
}

type Metrics []Metric

type Info struct {
	States  []State `json:"states"`
	Events  []Event `json:"events"`
	Metrics Metrics `json:"metrics"`
}

type RepairActionType string

const (
	// RepairActionTypeIgnoreNoActionRequired represents a suggested action to ignore the issue,
	// meaning no action is needed until further notice.
	RepairActionTypeIgnoreNoActionRequired RepairActionType = "IGNORE_NO_ACTION_REQUIRED"

	// RepairActionTypeRebootSystem represents a suggested action to reboot the system.
	// Specific to NVIDIA GPUs, this implies GPU reset by rebooting the system.
	RepairActionTypeRebootSystem RepairActionType = "REBOOT_SYSTEM"

	// RepairActionTypeHardwareInspection represents a suggested action for hardware inspection
	// and repair if any issue is found. This often involves data center (or cloud provider) support
	// to physically check/repair the machine.
	RepairActionTypeHardwareInspection RepairActionType = "HARDWARE_INSPECTION"

	// RepairActionTypeCheckUserApp represents a suggested action to check the user application.
	// For instance, NVIDIA may report XID 45 as user app error, but the underlying GPU might have other issues
	// thus requires further diagnosis of the application and the GPU.
	RepairActionTypeCheckUserAppAndGPU RepairActionType = "CHECK_USER_APP_AND_GPU"
)

// SuggestedActions represents a set of suggested actions to mitigate an issue.
type SuggestedActions struct {
	// References to the descriptions.
	References []string `json:"references,omitempty"`

	// A list of reasons and descriptions for the suggested actions.
	Descriptions []string `json:"descriptions"`

	// A list of repair actions to mitigate the issue.
	RepairActions []RepairActionType `json:"repair_actions"`
}

func (sa *SuggestedActions) DescribeActions() string {
	acts := make([]string, 0)
	for _, act := range sa.RepairActions {
		acts = append(acts, string(act))
	}
	return strings.Join(acts, ", ")
}

type EventType string

const (
	EventTypeUnknown EventType = "Unknown"

	// EventTypeInfo represents a general event that requires no action.
	// Info - Informative, no further action needed.
	EventTypeInfo EventType = "Info"

	// EventTypeWarning represents an event that may impact workloads.
	// Warning - Some issue happened but no further action needed, expecting automatic recovery.
	EventTypeWarning EventType = "Warning"

	// EventTypeCritical represents an event that is definitely impacting workloads
	// and requires immediate attention.
	// Critical - Some critical issue happened thus action required, not a hardware issue.
	EventTypeCritical EventType = "Critical"

	// EventTypeFatal represents a fatal event that impacts wide systems
	// and requires immediate attention and action.
	// Fatal - Fatal/hardware issue occurred thus immediate action required, may require reboot/hardware repair.
	EventTypeFatal EventType = "Fatal"
)

func EventTypeFromString(s string) EventType {
	switch s {
	case "Info":
		return EventTypeInfo
	case "Warning":
		return EventTypeWarning
	case "Critical":
		return EventTypeCritical
	case "Fatal":
		return EventTypeFatal
	default:
		return EventTypeUnknown
	}
}
