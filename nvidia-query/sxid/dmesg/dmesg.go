package dmesg

import (
	"regexp"
	"strconv"

	"github.com/leptonai/gpud/nvidia-query/sxid"
)

const (
	// e.g.,
	// [111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)
	// [131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up
	//
	// ref.
	// "D.4 Non-Fatal NVSwitch SXid Errors"
	// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
	RegexNVSwitchSXidDmesg = `SXid.*?: (\d+),`

	// Regex to extract PCI device ID from NVSwitch SXid messages
	RegexNVSwitchSXidDeviceUUID = `SXid \((PCI:[0-9a-fA-F:\.]+)\)`
)

var (
	compiledRegexNVSwitchSXidDmesg      = regexp.MustCompile(RegexNVSwitchSXidDmesg)
	compiledRegexNVSwitchSXidDeviceUUID = regexp.MustCompile(RegexNVSwitchSXidDeviceUUID)
)

// Extracts the nvidia NVSwitch SXid error code from the dmesg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
func ExtractNVSwitchSXid(line string) int {
	if match := compiledRegexNVSwitchSXidDmesg.FindStringSubmatch(line); match != nil {
		if id, err := strconv.Atoi(match[1]); err == nil {
			return id
		}
	}
	return 0
}

// ExtractNVSwitchSXidDeviceUUID extracts the PCI device ID from the dmesg log line.
// Returns empty string if the device ID is not found.
func ExtractNVSwitchSXidDeviceUUID(line string) string {
	if match := compiledRegexNVSwitchSXidDeviceUUID.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
}

type SXidError struct {
	SXid       int          `json:"sxid"`
	DeviceUUID string       `json:"device_uuid"`
	Detail     *sxid.Detail `json:"detail,omitempty"`
}

// Returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *SXidError {
	extractedID := ExtractNVSwitchSXid(line)
	if extractedID == 0 {
		return nil
	}
	detail, ok := sxid.GetDetail(extractedID)
	if !ok {
		return nil
	}
	deviceUUID := ExtractNVSwitchSXidDeviceUUID(line)
	return &SXidError{
		SXid:       extractedID,
		DeviceUUID: deviceUUID,
		Detail:     detail,
	}
}
