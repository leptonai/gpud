package dmesg

import (
	"regexp"
	"strconv"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
)

const (
	EventXid = "error_xid"

	// e.g.,
	// [...] NVRM: Xid (0000:03:00): 14, Channel 00000001
	// [...] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.
	// NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.
	//
	// ref.
	// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
	RegexNVRMXidDmesg = `NVRM: Xid.*?: (\d+),`

	// Regex to extract PCI device ID from NVRM Xid messages
	// Matches both formats: (0000:03:00) and (PCI:0000:05:00)
	RegexNVRMXidDeviceUUID = `NVRM: Xid \(((?:PCI:)?[0-9a-fA-F:]+)\)`
)

var (
	compiledRegexNVRMXidDmesg      = regexp.MustCompile(RegexNVRMXidDmesg)
	compiledRegexNVRMXidDeviceUUID = regexp.MustCompile(RegexNVRMXidDeviceUUID)
)

// Extracts the nvidia Xid error code from the dmesg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
func ExtractNVRMXid(line string) int {
	if match := compiledRegexNVRMXidDmesg.FindStringSubmatch(line); match != nil {
		if id, err := strconv.Atoi(match[1]); err == nil {
			return id
		}
	}
	return 0
}

// ExtractNVRMXidDeviceUUID extracts the PCI device ID from the NVRM Xid dmesg log line.
// For input without "PCI:" prefix, it returns the ID as is.
// For input with "PCI:" prefix, it returns the full ID including the prefix.
// Returns empty string if the device ID is not found.
func ExtractNVRMXidDeviceUUID(line string) string {
	if match := compiledRegexNVRMXidDeviceUUID.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
}

type XidError struct {
	Xid        int
	DeviceUUID string
	Detail     *xid.Detail
}

// Returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *XidError {
	extractedID := ExtractNVRMXid(line)
	if extractedID == 0 {
		return nil
	}
	detail, ok := xid.GetDetail(extractedID)
	if !ok {
		return nil
	}
	deviceUUID := ExtractNVRMXidDeviceUUID(line)
	return &XidError{
		Xid:        extractedID,
		DeviceUUID: deviceUUID,
		Detail:     detail,
	}
}
