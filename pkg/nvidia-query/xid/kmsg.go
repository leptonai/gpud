package xid

import (
	"regexp"
	"strconv"

	"github.com/leptonai/gpud/pkg/nvidia-query/xid"
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
	Xid        int         `json:"xid"`
	DeviceUUID string      `json:"device_uuid"`
	Detail     *xid.Detail `json:"detail,omitempty"`
}

// Match returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *XidError {
	extractedID, deviceUUID := ExtractNVRMXidInfo(line)
	if extractedID == 0 {
		return nil
	}
	detail, ok := xid.GetDetail(extractedID)
	if !ok {
		return nil
	}
	return &XidError{
		Xid:        extractedID,
		DeviceUUID: deviceUUID,
		Detail:     detail,
	}
}
