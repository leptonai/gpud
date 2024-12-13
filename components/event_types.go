package components

type EventType string

const (
	EventTypeUnknown EventType = "Unknown"

	// EventTypeInfo represents a general event that requires no action
	EventTypeInfo EventType = "Info"

	// EventTypeWarning represents an event that may impact workloads
	EventTypeWarning EventType = "Warning"

	// EventTypeCritical represents an event that is definitely impacting workloads
	// and requires immediate attention
	EventTypeCritical EventType = "Critical"

	// EventTypeFatal represents a fatal event that impacts wide systems
	// and requires immediate attention and action
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
