package xid

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

	// Extended regex for NVLink5 errors (XIDs 144-150) with subcode information
	// Captures: PCI address, XID, optional pid/name, subcode name, severity, XC type, injection, link, intrinfo, errorstatus
	// Example: NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 ...)
	// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
	RegexNVRMXidExtended = `NVRM: Xid \(PCI:([0-9a-fA-F:]+)\): (\d+)(?:, pid=(\d+), name=([^,]+))?, ([A-Z_]+(?:/[A-Z_]+)?)\s+(Nonfatal|Fatal)\s+(XC[01])\s+(i\d+)\s+Link\s+(-?\d+)\s+\((0x[0-9a-fA-F]+)\s+(0x[0-9a-fA-F]+)(?:\s+(0x[0-9a-fA-F]+))?(?:\s+(0x[0-9a-fA-F]+))?(?:\s+(0x[0-9a-fA-F]+))?(?:\s+(0x[0-9a-fA-F]+))?`

	// Fallback signatures for "GPU has fallen off the bus" logs that do not include
	// explicit "NVRM: Xid (...)" text.
	//
	// Multiline example:
	// NVRM: The NVIDIA GPU 0000:18:00.0
	// NVRM: ... fallen off the bus and is not responding to commands.
	RegexNVRMFallenOffBusMultiline = `(?s)NVRM:\s+The NVIDIA GPU ((?:[0-9a-fA-F]{4}:)?[0-9a-fA-F]{2}:[0-9a-fA-F]{2})\.0.*?fallen off the bus and is not responding to commands\.`

	// Single-line example:
	// NVRM: GPU 0000:18:00.0: GPU has fallen off the bus.
	RegexNVRMFallenOffBusSingleLine = `NVRM:\s+GPU ((?:[0-9a-fA-F]{4}:)?[0-9a-fA-F]{2}:[0-9a-fA-F]{2})\.0:\s+GPU has fallen off the bus\.?`
)

var (
	compiledRegexNVRMXidCombined            = regexp.MustCompile(RegexNVRMXidCombined)
	compiledRegexNVRMXidExtended            = regexp.MustCompile(RegexNVRMXidExtended)
	compiledRegexNVRMFallenOffBusMultiline  = regexp.MustCompile(RegexNVRMFallenOffBusMultiline)
	compiledRegexNVRMFallenOffBusSingleLine = regexp.MustCompile(RegexNVRMFallenOffBusSingleLine)
)

const (
	nvrmXidRegexIdxDeviceUUID = 1 + iota
	nvrmXidRegexIdxXid
	nvrmXidRegexIdxPid
	nvrmXidRegexIdxProcessName
	nvrmXidRegexIdxSubCodeName
	nvrmXidRegexIdxSeverity
	nvrmXidRegexIdxCrossContainment
	nvrmXidRegexIdxInjected
	nvrmXidRegexIdxLink
	nvrmXidRegexIdxIntrinfo
	nvrmXidRegexIdxErrorStatus
	nvrmXidRegexIdxOptionalHexStart
)

const nvrmXidRegexIdxOptionalHexEnd = nvrmXidRegexIdxOptionalHexStart + 3

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

// XidExtractedInfo contains detailed information extracted from NVLink5 XID error logs (XIDs 144-150).
// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
type XidExtractedInfo struct {
	DeviceUUID          string   `json:"device_uuid"`                     // PCI device address (e.g., "0018:01:00")
	Xid                 int      `json:"xid"`                             // XID error code (144-150 for NVLink5)
	Pid                 string   `json:"pid,omitempty"`                   // Process ID (optional, may be empty)
	ProcessName         string   `json:"process_name,omitempty"`          // Process name (optional, may be empty)
	SubCodeName         string   `json:"sub_code_name"`                   // Subcode mnemonic (e.g., "NETIR_LINK_EVT", "RLW_RXPIPE", "RLW_SRC_TRACK")
	Severity            string   `json:"severity"`                        // "Fatal" or "Nonfatal"
	CrossContainment    string   `json:"cross_containment"`               // "XC0" or "XC1" (cross-containment domain)
	Injected            string   `json:"injected"`                        // Injection status (e.g., "i0")
	Link                int      `json:"link"`                            // NVLink number (-1 for general errors)
	Intrinfo            uint32   `json:"intrinfo"`                        // First hex value - used for subcode calculation
	ErrorStatus         uint32   `json:"error_status"`                    // Second hex value - error status bits
	AdditionalHexValues []uint32 `json:"additional_hex_values,omitempty"` // Up to 4 additional hex values
	SubCode             int      `json:"sub_code"`                        // Calculated from bits 20-25 of intrinfo
}

// ExtractNVRMXidInfoExtended extracts detailed information from NVLink5 XID error logs (XIDs 144-150).
// This function parses extended log format with subcode information.
// Example log line:
//
//	NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 ...)
//
// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
// Returns nil if the line doesn't match the extended format.
func ExtractNVRMXidInfoExtended(line string) *XidExtractedInfo {
	match := compiledRegexNVRMXidExtended.FindStringSubmatch(line)
	if match == nil {
		return nil
	}

	if len(match) <= nvrmXidRegexIdxErrorStatus {
		return nil
	}

	xidCode, err := strconv.Atoi(match[nvrmXidRegexIdxXid])
	if err != nil {
		return nil
	}

	// Parse intrinfo (first hex value)
	intrinfoStr := match[nvrmXidRegexIdxIntrinfo]
	intrinfo, err := strconv.ParseUint(intrinfoStr, 0, 32)
	if err != nil {
		return nil
	}

	// Parse errorstatus (second hex value)
	errorStatusStr := match[nvrmXidRegexIdxErrorStatus]
	errorStatus, err := strconv.ParseUint(errorStatusStr, 0, 32)
	if err != nil {
		return nil
	}

	// Parse link number
	link, err := strconv.Atoi(match[nvrmXidRegexIdxLink])
	if err != nil {
		return nil
	}

	// Calculate subcode from bits 20-25 of intrinfo
	// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
	// NVLink5 errors encode sub-type identifiers in bits 20-25 of the intrinfo field.
	// Formula: idx = (intrinfo >> 20) & 0x3F
	subCode := calculateSubCode(uint32(intrinfo))

	// Parse optional additional hex values (indices 12-15)
	var additionalHexValues []uint32
	for i := nvrmXidRegexIdxOptionalHexStart; i <= nvrmXidRegexIdxOptionalHexEnd && i < len(match); i++ {
		if match[i] == "" {
			continue
		}
		if val, err := strconv.ParseUint(match[i], 0, 32); err == nil {
			additionalHexValues = append(additionalHexValues, uint32(val))
		}
	}

	return &XidExtractedInfo{
		DeviceUUID:          match[nvrmXidRegexIdxDeviceUUID],
		Xid:                 xidCode,
		Pid:                 match[nvrmXidRegexIdxPid],         // May be empty
		ProcessName:         match[nvrmXidRegexIdxProcessName], // May be empty
		SubCodeName:         match[nvrmXidRegexIdxSubCodeName],
		Severity:            match[nvrmXidRegexIdxSeverity],
		CrossContainment:    match[nvrmXidRegexIdxCrossContainment],
		Injected:            match[nvrmXidRegexIdxInjected],
		Link:                link,
		Intrinfo:            uint32(intrinfo),
		ErrorStatus:         uint32(errorStatus),
		AdditionalHexValues: additionalHexValues,
		SubCode:             subCode,
	}
}

// calculateSubCode extracts the subcode from the intrinfo field for NVLink5 XIDs (144-150).
// The subcode is encoded in bits 20-25 of the intrinfo field.
// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
// Formula: idx = (intrinfo >> 20) & 0x3F
func calculateSubCode(intrinfo uint32) int {
	return int((intrinfo >> 20) & 0x3F)
}

type XidError struct {
	Xid        int     `json:"xid"`
	DeviceUUID string  `json:"device_uuid"`
	Detail     *Detail `json:"detail,omitempty"`
}

// Match returns a matching xid error object if found.
// Otherwise, returns nil.
func Match(line string) *XidError {
	if info := ExtractNVRMXidInfoExtended(line); info != nil {
		if detail, ok := detailFromNVLinkInfo(info); ok {
			deviceUUID := info.DeviceUUID
			if deviceUUID != "" && !strings.HasPrefix(deviceUUID, "PCI:") {
				deviceUUID = "PCI:" + deviceUUID
			}
			return &XidError{
				Xid:        info.Xid,
				DeviceUUID: deviceUUID,
				Detail:     detail,
			}
		}
	}

	extractedID, deviceUUID := ExtractNVRMXidInfo(line)
	if extractedID != 0 {
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

	if xid, fallbackDeviceUUID := extractFallenOffBusXidInfo(line); xid != 0 {
		// NVIDIA docs map "GPU has fallen off the bus" to XID 79 even when newer
		// driver logs omit the explicit "Xid (...): 79" prefix.
		detail, ok := GetDetail(xid)
		if !ok {
			return nil
		}
		return &XidError{
			Xid:        xid,
			DeviceUUID: fallbackDeviceUUID,
			Detail:     detail,
		}
	}

	return nil
}

func extractFallenOffBusXidInfo(line string) (int, string) {
	if match := compiledRegexNVRMFallenOffBusSingleLine.FindStringSubmatch(line); len(match) == 2 {
		return 79, normalizePCIBDF(match[1])
	}

	if match := compiledRegexNVRMFallenOffBusMultiline.FindStringSubmatch(line); len(match) == 2 {
		return 79, normalizePCIBDF(match[1])
	}

	return 0, ""
}

func normalizePCIBDF(deviceUUID string) string {
	normalized := strings.TrimSpace(strings.TrimPrefix(deviceUUID, "PCI:"))
	if strings.Count(normalized, ":") == 1 {
		normalized = "0000:" + normalized
	}
	if normalized == "" {
		return ""
	}
	return "PCI:" + normalized
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
