package xid

import (
	"encoding/json"

	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
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

	// RawEvent is the raw event from the nvml API.
	RawEvent *nvidia_query_nvml.XidEvent `json:"raw_event,omitempty"`

	// Xid is the corresponding XID from the raw event.
	// The monitoring component can use this Xid to decide its own action.
	Xid uint64 `json:"xid"`

	// XidCriticalErrorMarkedByNVML is true if the NVML marks this error as a critical error.
	XidCriticalErrorMarkedByNVML bool `json:"xid_critical_error_marked_by_nvml"`

	// XidCriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	XidCriticalErrorMarkedByGPUd bool `json:"xid_critical_error_marked_by_gpud"`
}
