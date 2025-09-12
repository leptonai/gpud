package sxid

import (
	"regexp"
	"strconv"
)

const (
	// e.g.,
	// [111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)
	// [131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up
	//
	// ref.
	// "D.4 Non-Fatal NVSwitch SXid Errors"
	// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
	RegexNVSwitchSXidKMessage = `SXid.*?: (\d+),`

	// Regex to extract PCI device ID from NVSwitch SXid messages
	RegexNVSwitchSXidDeviceUUID = `SXid \((PCI:[0-9a-fA-F:\.]+)\)`
)

var (
	compiledRegexNVSwitchSXidKMessage   = regexp.MustCompile(RegexNVSwitchSXidKMessage)
	compiledRegexNVSwitchSXidDeviceUUID = regexp.MustCompile(RegexNVSwitchSXidDeviceUUID)
)

// ExtractNVSwitchSXid extracts the nvidia NVSwitch SXid error code from the kmsg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
func ExtractNVSwitchSXid(line string) int {
	if match := compiledRegexNVSwitchSXidKMessage.FindStringSubmatch(line); match != nil {
		if id, err := strconv.Atoi(match[1]); err == nil {
			return id
		}
	}
	return 0
}

// ExtractNVSwitchSXidDeviceUUID extracts the PCI device ID from the kmsg log line.
// Returns empty string if the device ID is not found.
func ExtractNVSwitchSXidDeviceUUID(line string) string {
	if match := compiledRegexNVSwitchSXidDeviceUUID.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
}

type SXidError struct {
	SXid       int     `json:"sxid"`
	DeviceUUID string  `json:"device_uuid"`
	Detail     *Detail `json:"detail,omitempty"`
}

// Match returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *SXidError {
	extractedID := ExtractNVSwitchSXid(line)
	if extractedID == 0 {
		return nil
	}
	detail, ok := GetDetail(extractedID)
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
