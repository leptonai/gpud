package v1

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HealthStateType string

const (
	HealthStateTypeHealthy      HealthStateType = "Healthy"
	HealthStateTypeUnhealthy    HealthStateType = "Unhealthy"
	HealthStateTypeDegraded     HealthStateType = "Degraded"
	HealthStateTypeInitializing HealthStateType = "Initializing"
)

// HealthState represents the health state of a component.
// The healthiness of the component is already evaluated at the component level,
// so the health state here is to provide more details about the healthiness,
// and other data for the control plane to decide how to alert and remediate the issue.
type HealthState struct {
	// Component represents which component generated the state.
	Component string `json:"component,omitempty"`

	// Name is the name of the state.
	Name string `json:"name,omitempty"`

	// Health represents the health level of the state,
	// including StateHealthy, StateUnhealthy and StateDegraded.
	// StateDegraded is similar to Unhealthy which also can trigger alerts
	// for users or operators, but what StateDegraded means is that the
	// issue detected does not affect users’ workload.
	Health HealthStateType `json:"health,omitempty"`

	// Reason represents what happened or detected by GPUd if it isn’t healthy.
	Reason string `json:"reason,omitempty"`

	// Error represents the detailed error information, which will be shown
	// as More Information to help analyze why it isn’t healthy.
	Error string `json:"error,omitempty"`

	// SuggestedActions represents the suggested actions to mitigate the issue.
	SuggestedActions *SuggestedActions `json:"suggested_actions,omitempty"`

	ExtraInfo map[string]string `json:"extra_info,omitempty"`
}

type HealthStates []HealthState

type ComponentHealthStates struct {
	Component string       `json:"component"`
	States    HealthStates `json:"states"`
}

type GPUdComponentHealthStates []ComponentHealthStates

// Event represents an event that happened in a component at a specific time.
// A single event itself does not dictate whether the component is healthy or not.
// The healthiness of the component is evaluated at the component health state level.
type Event struct {
	// Component represents which component generated the event.
	Component string `json:"component,omitempty"`

	// Time represents when the event happened.
	Time metav1.Time `json:"time"`

	// Name represents the name of the event.
	Name string `json:"name,omitempty"`

	// Type represents the type of the event.
	Type EventType `json:"type,omitempty"`

	// Message represents the detailed message of the event.
	Message string `json:"message,omitempty"`

	// TO BE DEPRECATED
	DeprecatedExtraInfo        map[string]string `json:"extra_info,omitempty"`
	DeprecatedSuggestedActions *SuggestedActions `json:"suggested_actions,omitempty"`
}

type Events []Event

type ComponentEvents struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Events    Events    `json:"events"`
}

type GPUdComponentEvents []ComponentEvents

type Metric struct {
	UnixSeconds int64 `json:"unix_seconds"`

	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`

	// TO BE DEPRECATED
	DeprecatedMetricName          string `json:"metric_name"`
	DeprecatedMetricSecondaryName string `json:"metric_secondary_name,omitempty"`
}

type Metrics []Metric

type ComponentMetrics struct {
	Component string  `json:"component"`
	Metrics   Metrics `json:"metrics"`
}

type GPUdComponentMetrics []ComponentMetrics

type Info struct {
	States  HealthStates `json:"states"`
	Events  Events       `json:"events"`
	Metrics Metrics      `json:"metrics"`
}

type ComponentInfo struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Info      Info      `json:"info"`
}

type GPUdComponentInfos []ComponentInfo

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
	// Description describes the issue in detail.
	Description string `json:"description"`

	// A list of repair actions to mitigate the issue.
	RepairActions []RepairActionType `json:"repair_actions"`

	// TO BE DEPRECATED
	DeprecatedReferences   []string `json:"references,omitempty"`
	DeprecatedDescriptions []string `json:"descriptions,omitempty"`
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

type MachineInfo struct {
	// GPUdVersion represents the current version of GPUd
	GPUdVersion string `json:"gpudVersion,omitempty"`

	// GPUDriverVersion represents the current version of GPU driver installed
	GPUDriverVersion string `json:"gpuDriverVersion,omitempty"`
	// CUDAVersion represents the current version of cuda library.
	CUDAVersion string `json:"cudaVersion,omitempty"`
	// ContainerRuntime Version reported by the node through runtime remote API (e.g. containerd://1.4.2).
	ContainerRuntimeVersion string `json:"containerRuntimeVersion,omitempty"`
	// Kernel Version reported by the node from 'uname -r' (e.g. 3.16.0-0.bpo.4-amd64).
	KernelVersion string `json:"kernelVersion,omitempty"`
	// OS Image reported by the node from /etc/os-release (e.g. Debian GNU/Linux 7 (wheezy)).
	OSImage string `json:"osImage,omitempty"`
	// The Operating System reported by the node
	OperatingSystem string `json:"operatingSystem,omitempty"`
	// SystemUUID comes from https://github.com/google/cadvisor/blob/master/utils/sysfs/sysfs.go#L442
	SystemUUID string `json:"systemUUID,omitempty"`
	// MachineID is collected by GPUd. It comes from /etc/machine-id or /var/lib/dbus/machine-id
	MachineID string `json:"machineID,omitempty"`
	// BootID is collected by GPUd.
	BootID string `json:"bootID,omitempty"`
	// Uptime represents when the machine up
	Uptime metav1.Time `json:"uptime,omitempty"`

	CPUInfo *MachineCPUInfo `json:"cpuInfo"`
	GPUInfo *MachineGPUInfo `json:"gpuInfo"`
}

type MachineCPUInfo struct {
	Type         string `json:"type,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

type MachineGPUInfo struct {
	Product      string `json:"product,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Memory       string `json:"memory,omitempty"`
}

type MachineNetwork struct {
	PublicIP  string `json:"publicIP,omitempty"`
	PrivateIP string `json:"privateIP,omitempty"`
}

type MachineLocation struct {
	Region string `json:"region,omitempty"`
	Zone   string `json:"zone,omitempty"`
}
