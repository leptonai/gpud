package common

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
