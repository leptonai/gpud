package v1

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

type LeptonEvents []LeptonComponentEvents
type LeptonStates []LeptonComponentStates
type LeptonMetrics []LeptonComponentMetrics
type LeptonInfo []LeptonComponentInfo

type LeptonComponentEvents struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Events    []Event   `json:"events"`
}

type LeptonComponentStates struct {
	Component string  `json:"component"`
	States    []State `json:"states"`
}

type LeptonComponentMetrics struct {
	Component string  `json:"component"`
	Metrics   Metrics `json:"metrics"`
}

type LeptonComponentInfo struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Info      Info      `json:"info"`
}
