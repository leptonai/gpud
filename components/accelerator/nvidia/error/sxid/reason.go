package sxid

import (
	"encoding/json"

	"github.com/leptonai/gpud/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reason defines the reason for the output evaluation in the JSON format.
type Reason struct {
	// Messages are the messages for the reason.
	// And do not include the errors.
	Messages []string `json:"messages"`

	// Errors are the sxid errors that happened, sorted by the event time in ascending order.
	Errors []SXidError `json:"errors"`
}

func (r Reason) JSON() ([]byte, error) {
	return json.Marshal(r)
}

// SXidError represents an SXid error in the reason.
type SXidError struct {
	// Time is the time of the event.
	Time metav1.Time `json:"time"`

	// DataSource is the source of the data.
	DataSource string `json:"data_source"`

	// DeviceUUID is the UUID of the device that has the error.
	DeviceUUID string `json:"device_uuid"`

	// SXid is the corresponding SXid from the raw event.
	// The monitoring component can use this SXid to decide its own action.
	SXid uint64 `json:"sxid"`

	// SuggestedActionsByGPUd are the suggested actions for the error.
	SuggestedActionsByGPUd *common.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
}

func (sxidErr SXidError) JSON() ([]byte, error) {
	return json.Marshal(sxidErr)
}
