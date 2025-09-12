package xid

import (
	"fmt"
	"regexp"
	"strconv"
)

const (
	// e.g.,
	// [...] NVRM: Xid (0000:03:00): 14, Channel 00000001
	// [...] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.
	// NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.
	//
	// ref.
	// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf

	// Combined regex to extract both device UUID and Xid error code in one match
	// Group 1: Device UUID (with or without PCI: prefix)
	// Group 2: Xid error code
	RegexNVRMXidCombined = `NVRM: Xid \(((?:PCI:)?[0-9a-fA-F:]+)\).*?: (\d+),`
)

var (
	compiledRegexNVRMXidCombined = regexp.MustCompile(RegexNVRMXidCombined)
)

// ExtractNVRMXidInfo extracts both the nvidia Xid error code and device UUID from the dmesg log line.
// Returns (xidCode, deviceUUID) or (0, "") if not found.
// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
func ExtractNVRMXidInfo(line string) (int, string) {
	if match := compiledRegexNVRMXidCombined.FindStringSubmatch(line); match != nil {
		if id, err := strconv.Atoi(match[2]); err == nil {
			return id, match[1]
		}
	}
	return 0, ""
}

// ExtractNVRMXid extracts the nvidia Xid error code from the dmesg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
func ExtractNVRMXid(line string) int {
	id, _ := ExtractNVRMXidInfo(line)
	return id
}

type XidError struct {
	Xid        int     `json:"xid"`
	DeviceUUID string  `json:"device_uuid"`
	Detail     *Detail `json:"detail,omitempty"`
}

// Match returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *XidError {
	extractedID, deviceUUID := ExtractNVRMXidInfo(line)
	if extractedID == 0 {
		return nil
	}
	detail, ok := GetDetail(extractedID)
	if !ok {
		return nil
	}
	return &XidError{
		Xid:        extractedID,
		DeviceUUID: deviceUUID,
		Detail:     detail,
	}
}

// MessageToInject represents a synthetic kernel message snippet and its log priority.
type MessageToInject struct {
	Priority string
	Message  string
}

// GetMessageToInject returns an example NVRM Xid dmesg line for a given XID.
// If the XID is not recognized, it returns a generic placeholder.
func GetMessageToInject(xid int) MessageToInject {
	msg, ok := xidExampleMsgs[xid]
	if !ok {
		return MessageToInject{
			Priority: "KERN_WARNING",
			Message:  fmt.Sprintf("NVRM: Xid (PCI:0000:04:00): %d, unknown", xid),
		}
	}
	return msg
}

// Example messages for common/known XIDs used in tests or tooling.
var xidExampleMsgs = map[int]MessageToInject{
	63: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 63, Row remapping event: Rows 0x1a and 0x2b have been remapped on GPU 00000000:04:00.0",
	},
	64: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 64, Failed to persist row remap table — GPU may require servicing",
	},
	69: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 69, pid=34566, name=python3, BAR1 access failure at address 0xffff80001234abcd",
	},
	74: {
		Priority: "KERN_WARNING",
		Message:  "NVRM: Xid (PCI:0000:04:00): 74, pid=1234, name=python3, Channel 0x23, MMU Fault: ENGINE GRAPHICS GPCCLIENT_T1_0 faulted @ 0x7fc123456000. Fault is of type FAULT_PTE ACCESS_TYPE_VIRT_READ",
	},
	79: {
		Priority: "KERN_ERR",
		Message:  "NVRM: Xid (PCI:0000:04:00): 79, GPU has fallen off the bus",
	},
}
