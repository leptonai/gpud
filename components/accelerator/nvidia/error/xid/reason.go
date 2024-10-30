package xid

import (
	"encoding/json"

	"github.com/leptonai/gpud/components/common"
)

// Reason defines the reason for the output evaluation in the JSON format.
type Reason struct {
	// Messages are the messages for the reason.
	// And do not include the errors.
	Messages []string `json:"messages"`

	// Errors are the xid errors that happened, keyed by the XID.
	Errors map[uint64]XidError `json:"errors"`

	// OtherErrors are other errors that happened during the evaluation.
	OtherErrors []string `json:"other_errors,omitempty"`
}

func (r Reason) JSON() ([]byte, error) {
	return json.Marshal(r)
}

// XidError represents an XID error in the reason.
type XidError struct {
	// DataSource is the source of the data.
	DataSource string `json:"data_source"`

	// DeviceUUID is the UUID of the device that has the error.
	DeviceUUID string `json:"device_uuid"`

	// Xid is the corresponding XID from the raw event.
	// The monitoring component can use this Xid to decide its own action.
	Xid uint64 `json:"xid"`

	// Description is the description of the error.
	XidDescription string `json:"xid_description"`

	// XidCriticalErrorMarkedByNVML is true if the NVML marks this error as a critical error.
	XidCriticalErrorMarkedByNVML bool `json:"xid_critical_error_marked_by_nvml"`

	// XidCriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	XidCriticalErrorMarkedByGPUd bool `json:"xid_critical_error_marked_by_gpud"`

	// SuggestedActions are the suggested actions for the error.
	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`
}
