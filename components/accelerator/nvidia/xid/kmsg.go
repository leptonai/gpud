package xid

import (
	"fmt"
	"regexp"
	"strings"

	xidsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg/xid"
)

var (
	compiledRegexNVRMFallenOffBusMultiline  = regexp.MustCompile(xidsignals.DefaultPatterns()[xidsignals.NVRMFallenOffBusMultiline])
	compiledRegexNVRMFallenOffBusSingleLine = regexp.MustCompile(xidsignals.DefaultPatterns()[xidsignals.NVRMFallenOffBusSingleLine])
)

// nvlinkInfo contains the facts needed by GPUd's NVLink recovery policy.
// Parsing the driver log is owned by the health-validation Xid parser.
type nvlinkInfo struct {
	Xid         int
	SubCodeName string
	Severity    string
	Intrinfo    uint32
	ErrorStatus uint32
	SubCode     int
}

// calculateSubCode extracts the subcode from the intrinfo field for NVLink5 XIDs (144-150).
// The subcode is encoded in bits 20-25 of the intrinfo field.
// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
// Formula: idx = (intrinfo >> 20) & 0x3F
func calculateSubCode(intrinfo uint32) int {
	return int((intrinfo >> 20) & 0x3F)
}

// Error describes a parsed XID kernel message.
type Error struct {
	Xid        int     `json:"xid"`
	DeviceUUID string  `json:"device_uuid"`
	Detail     *Detail `json:"detail,omitempty"`
}

// errorFromFacts applies GPUd's catalog and recovery policy to parsed facts.
func errorFromFacts(code int64, address string, extended *nvlinkInfo) *Error {
	xid := int(code)
	if int64(xid) != code {
		return nil
	}
	detail, ok := GetDetail(xid)
	if !ok {
		return nil
	}
	if extended != nil {
		extended.Xid = xid
		if decoded, ok := detailFromNVLinkInfo(extended); ok {
			detail = decoded
		}
	}
	return &Error{Xid: xid, DeviceUUID: "PCI:" + address, Detail: detail}
}

// A fallen-off-bus line without an explicit Xid is interpreted as Xid 79 by
// GPUd policy. It is not a parsed Xid fact from the sensor.
func fallenOffBusError(line string) *Error {
	code, device := extractFallenOffBusXidInfo(line)
	if code == 0 {
		return nil
	}
	detail, ok := GetDetail(code)
	if !ok {
		return nil
	}
	return &Error{Xid: code, DeviceUUID: device, Detail: detail}
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
