// Package xid provides the NVIDIA XID error details.
package xid

import (
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Detail describes a static XID catalog entry.
type Detail struct {
	// Code is the error code of the Xid error, as documented in
	// https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html.
	Code int `json:"code"`

	// Description is the description of the Xid error, as documented in
	// https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html.
	Description string `json:"description"`

	// SubCode is populated for NVLink (144-150) XIDs after decoding intrinfo bits 20-25.
	SubCode int `json:"sub_code"`
	// SubCodeDescription describes the NVLink sub-component (e.g., NETIR_LINK_EVT).
	SubCodeDescription string `json:"sub_code_description"`

	// ErrorStatus is the NVLink error status word associated with the decoded rule (if applicable).
	ErrorStatus uint32 `json:"error_status,omitempty"`

	// InvestigatoryHint is a short, user-friendly hint derived from the NVLink rule's
	// Investigatory field. It helps differentiate errors that have the same Unit but
	// different root causes (e.g., "peer" vs "software" for NETIR_LINK_EVT errors).
	InvestigatoryHint string `json:"investigatory_hint,omitempty"`

	// SuggestedActionsByGPUd is the suggested actions by GPUd.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`

	// EventType is the type of the event.
	// The xid component health state is set to "Unhealthy"
	// if this event type is "Critical" or "Fatal".
	EventType apiv1.EventType `json:"event_type"`
}

type catalogEntry struct {
	Code                    int
	Mnemonic                string
	Description             string
	ImmediateResolution     string
	InvestigatoryResolution string
}

type nvlinkRule struct {
	Xid               int
	Unit              string
	IntrinfoPatternV1 string
	IntrinfoPatternV2 string
	ErrorStatus       uint32
	Resolution        string
	Investigatory     string
	Severity          string
	Action2           string
	HwSw              string
	LocalRemote       string
}

var (
	detailsWithSubCodes map[int]map[int]Detail
	// detailsWithSubCodesByStatus keeps rule-specific details keyed by XID -> subcode -> errorStatus.
	detailsWithSubCodesByStatus map[int]map[int]map[uint32]Detail
	nvlinkRulesByXID            = indexNVLinkRules()
)

// GetDetail returns the XID detail for the given code.
func GetDetail(id int) (*Detail, bool) {
	e, ok := details[id]
	if ok {
		result := evaluateGoHealth(id)
		// NVLink severity remains subcode/status-specific; all XID actions and
		// base-XID severity are owned by go-health.
		if !isNVLinkXID(id) {
			e.EventType = result.eventType
		}
		e.SuggestedActionsByGPUd = suggestedActionsFromGoHealth(result.recommendedAction)
	}
	return &e, ok
}

func isNVLinkXID(id int) bool {
	return id >= 144 && id <= 150
}

// getDetailWithSubCode returns the XID detail for a given base code and subcode.
// For XIDs 144-150, subcode information is derived from NVIDIA's NVLink catalog.
func getDetailWithSubCode(xid int, subCode int) (*Detail, bool) {
	if subMap, ok := detailsWithSubCodes[xid]; ok {
		if detail, ok := subMap[subCode]; ok {
			detailCopy := detail
			return &detailCopy, true
		}
		if detail, ok := subMap[0]; ok {
			detailCopy := detail
			return &detailCopy, true
		}
	}
	return GetDetail(xid)
}

// getDetailWithSubCodeAndStatus returns the XID detail for a given base code, subcode, and errorStatus
// for NVLink XIDs (144-150). Falls back progressively to subcode-only and then to base detail.
func getDetailWithSubCodeAndStatus(xid int, subCode int, errorStatus uint32) (*Detail, bool) {
	if statusMap, ok := detailsWithSubCodesByStatus[xid]; ok {
		if subMap, ok := statusMap[subCode]; ok {
			if detail, ok := subMap[errorStatus]; ok {
				detailCopy := detail
				return &detailCopy, true
			}
		}
	}
	return getDetailWithSubCode(xid, subCode)
}

func init() {
	detailsWithSubCodes, detailsWithSubCodesByStatus = buildNVLinkSubCodeDetails()
}

// Copied from https://docs.nvidia.com/deploy/xid-details/index.html#xid-error-listing.
// See https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages
// and https://docs.nvidia.com/deploy/xid-errors/index.html for more details.
var details = map[int]Detail{
	1: {
		Code:        1,
		Description: "Invalid or corrupted push buffer stream",
	},
	2: {
		Code:        2,
		Description: "Invalid or corrupted push buffer stream",
	},
	3: {
		Code:        3,
		Description: "Invalid or corrupted push buffer stream",
	},
	4: {
		Code:        4,
		Description: "Invalid or corrupted push buffer stream",
	},
	5: {
		Code:        5,
		Description: "Unused",
	},
	6: {
		Code:        6,
		Description: "Invalid or corrupted push buffer stream",
	},
	7: {
		Code:        7,
		Description: "Invalid or corrupted push buffer address",
	},
	8: {
		Code:        8,
		Description: "GPU stopped processing",
	},
	9: {
		Code:        9,
		Description: "Driver error programming GPU",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	10: {
		Code:        10,
		Description: "Unused",
	},
	11: {
		Code:        11,
		Description: "Invalid or corrupted push buffer stream",
	},
	12: {
		Code:        12,
		Description: "Driver error handling GPU exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	13: {
		Code:        13,
		Description: "Graphics Engine Exception",
	},
	14: {
		Code:        14,
		Description: "Unused",
	},
	15: {
		Code:        15,
		Description: "Unused",
	},
	16: {
		Code:        16,
		Description: "Display engine hung",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	17: {
		Code:        17,
		Description: "Unused",
	},
	18: {
		Code:        18,
		Description: "Bus mastering disabled in PCI Config Space",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	19: {
		Code:        19,
		Description: "Display Engine error",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	20: {
		Code:        20,
		Description: "Invalid or corrupted Mpeg push buffer",
	},
	21: {
		Code:        21,
		Description: "Invalid or corrupted Motion Estimation push buffer",
	},
	22: {
		Code:        22,
		Description: "Invalid or corrupted Video Processor push buffer",
	},
	23: {
		Code:        23,
		Description: "Unused",
	},
	24: {
		Code:        24,
		Description: "GPU semaphore timeout",
	},
	25: {
		Code:        25,
		Description: "Invalid or illegal push buffer stream",
	},
	26: {
		Code:        26,
		Description: "Framebuffer timeout",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	27: {
		Code:        27,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	28: {
		Code:        28,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	29: {
		Code:        29,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	30: {
		Code:        30,
		Description: "GPU semaphore access error",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	31: {
		Code:        31,
		Description: "GPU memory page fault",
	},
	32: {
		Code:        32,
		Description: "Invalid or corrupted push buffer stream",
	},
	33: {
		Code:        33,
		Description: "Internal micro-controller error",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	34: {
		Code:        34,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	35: {
		Code:        35,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	36: {
		Code:        36,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	37: {
		Code:        37,
		Description: "Driver firmware error",
	},
	38: {
		Code:        38,
		Description: "Driver firmware error",
	},
	39: {
		Code:        39,
		Description: "Unused",
	},
	40: {
		Code:        40,
		Description: "Unused",
	},
	41: {
		Code:        41,
		Description: "Unused",
	},
	42: {
		Code:        42,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	43: {
		Code:        43,
		Description: "GPU stopped processing",
	},
	44: {
		Code:        44,
		Description: "Graphics Engine fault during context switch",
	},
	45: {
		Code:        45,
		Description: "Preemptive cleanup, due to previous errors – Most likely to see when running multiple cuda applications and hitting a DBE.",
		// TODO
		// unhealthy if there's no previous Xid event in the same time window
	},
	46: {
		Code:        46,
		Description: "GPU stopped processing",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	47: {
		Code:        47,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	48: {
		Code:        48,
		Description: "Double Bit ECC Error",
	},
	49: {
		Code:        49,
		Description: "Unused",
	},
	50: {
		Code:        50,
		Description: "Unused",
	},
	51: {
		Code:        51,
		Description: "Unused",
	},
	52: {
		Code:        52,
		Description: "Unused",
	},
	53: {
		Code:        53,
		Description: "Unused",
	},
	54: {
		Code:        54,
		Description: "Auxiliary power is not connected to the GPU board",
	},
	55: {
		Code:        55,
		Description: "Unused",
	},
	56: {
		Code:        56,
		Description: "Display Engine error",
	},
	57: {
		Code:        57,
		Description: "Error programming video memory interface",
	},
	58: {
		Code:        58,
		Description: "Unstable video memory interface detected",
	},
	59: {
		Code:        59,
		Description: "Internal micro-controller error (older drivers)",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	60: {
		Code:        60,
		Description: "Video processor exception",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	61: {
		Code:        61,
		Description: "Internal micro-controller breakpoint/warning (newer drivers)",
	},
	62: {
		Code:        62,
		Description: "Internal micro-controller halt (newer drivers)",
	},
	63: {
		Code:        63,
		Description: "ECC page retirement or row remapping recording event",
	},
	64: {
		Code:        64,
		Description: "ECC page retirement or row remapper recording failure",
	},
	65: {
		Code:        65,
		Description: "Video processor exception",

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
	},
	66: {
		Code:        66,
		Description: "Illegal access by driver",
	},
	67: {
		Code:        67,
		Description: "Illegal access by driver",
	},
	68: {
		Code:        68,
		Description: "NVDEC0 Exception",

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		// TODO: verify whether this is still true https://github.com/NVIDIA/k8s-device-plugin/issues/945
	},
	69: {
		Code:        69,
		Description: "Graphics Engine class error",
	},
	70: {
		Code:        70,
		Description: "CE3: Unknown Error",
	},
	71: {
		Code:        71,
		Description: "CE4: Unknown Error",
	},
	72: {
		Code:        72,
		Description: "CE5: Unknown Error",
	},
	73: {
		Code:        73,
		Description: "NVENC2 Error",
	},
	74: {
		Code:        74,
		Description: "NVLINK Error",
	},
	75: {
		Code:        75,
		Description: "CE6: Unknown Error",
	},
	76: {
		Code:        76,
		Description: "CE7: Unknown Error",
	},
	77: {
		Code:        77,
		Description: "CE8: Unknown Error",
	},
	78: {
		Code:        78,
		Description: "vGPU Start Error",

		// if nvidia says this can be only because of driver error, then we only reboot
	},
	79: {
		Code:        79,
		Description: "GPU has fallen off the bus",
	},
	80: {
		Code:        80,
		Description: "Corrupted data sent to GPU",
	},
	81: {
		Code:        81,
		Description: "VGA Subsystem Error",

		// if nvidia says only possible reason is hw, then we do hard inspections directly
	},
	82: {
		Code:        82,
		Description: "NVJPG0 Error",
	},
	83: {
		Code:        83,
		Description: "NVDEC1 Error",
	},
	84: {
		Code:        84,
		Description: "NVDEC2 Error",
	},
	85: {
		Code:        85,
		Description: "CE9: Unknown Error",
	},
	86: {
		Code:        86,
		Description: "OFA Exception",
	},
	87: {
		Code:        87,
		Description: "Reserved",
	},
	88: {
		Code:        88,
		Description: "NVDEC3 Error",
	},
	89: {
		Code:        89,
		Description: "NVDEC4 Error",
	},
	90: {
		Code:        90,
		Description: "Reserved",
	},
	91: {
		Code:        91,
		Description: "Reserved",
	},
	92: {
		Code:        92,
		Description: "High single-bit ECC error rate",
	},
	93: {
		Code:        93,
		Description: "Non-fatal violation of provisioned InfoROM wear limit",
	},
	94: {
		Code:        94,
		Description: "Contained ECC error",
	},
	95: {
		Code:        95,
		Description: "Uncontained ECC error",
	},
	96: {
		Code:        96,
		Description: "NVDEC5 Error",
	},
	97: {
		Code:        97,
		Description: "NVDEC6 Error",
	},
	98: {
		Code:        98,
		Description: "NVDEC7 Error",
	},
	99: {
		Code:        99,
		Description: "NVJPG1 Error",
	},
	100: {
		Code:        100,
		Description: "NVJPG2 Error",
	},
	101: {
		Code:        101,
		Description: "NVJPG3 Error",
	},
	102: {
		Code:        102,
		Description: "NVJPG4 Error",
	},
	103: {
		Code:        103,
		Description: "NVJPG5 Error",
	},
	104: {
		Code:        104,
		Description: "NVJPG6 Error",
	},
	105: {
		Code:        105,
		Description: "NVJPG7 Error",
	},
	106: {
		Code:        106,
		Description: "SMBPBI Test Message",
	},
	107: {
		Code:        107,
		Description: "SMBPBI Test Message Silent",
	},
	108: {
		Code:        108,
		Description: "Reserved",
	},
	109: {
		Code:        109,
		Description: "Context Switch Timeout Error",
	},
	110: {
		Code:        110,
		Description: "Security Fault Error",

		// if nvidia says only possible reason is hw, then we do hard inspections directly
	},
	111: {
		Code:        111,
		Description: "Display Bundle Error Event",
	},
	112: {
		Code:        112,
		Description: "Display Supervisor Error",
	},
	113: {
		Code:        113,
		Description: "DP Link Training Erro",
	},
	114: {
		Code:        114,
		Description: "Display Pipeline Underflow Error",
	},
	115: {
		Code:        115,
		Description: "Display Core Channel Error",
	},
	116: {
		Code:        116,
		Description: "Display Window Channel Error",
	},
	117: {
		Code:        117,
		Description: "Display Cursor Channel Error",
	},
	118: {
		Code:        118,
		Description: "Display Pixel Pipeline Error",
	},
	119: {
		Code:        119,
		Description: "GSP RPC Timeout",
	},
	120: {
		Code:        120,
		Description: "GSP Error",
	},
	121: {
		Code:        121,
		Description: "C2C Link Error",
	},
	122: {
		Code:        122,
		Description: "SPI PMU RPC Read Failure",
	},
	123: {
		Code:        123,
		Description: "SPI PMU RPC Write Failure",
	},
	124: {
		Code:        124,
		Description: "SPI PMU RPC Erase Failure",
	},
	125: {
		Code:        125,
		Description: "Inforom FS Failure",
	},
	126: {
		Code:        126,
		Description: "Reserved",
	},
	127: {
		Code:        127,
		Description: "Reserved",
	},
	128: {
		Code:        128,
		Description: "Reserved",
	},
	129: {
		Code:        129,
		Description: "Reserved",
	},
	130: {
		Code:        130,
		Description: "Reserved",
	},
	131: {
		Code:        131,
		Description: "Reserved",
	},
	132: {
		Code:        132,
		Description: "Reserved",
	},
	134: {
		Code:        134,
		Description: "Reserved",
	},
	135: {
		Code:        135,
		Description: "Reserved",
	},
	136: {
		Code:        136,
		Description: "Reserved",
	},
	137: {
		Code:        137,
		Description: "NVLink FLA privilege error",
	},
	138: {
		Code:        138,
		Description: "Reserved",
	},
	139: {
		Code:        139,
		Description: "Reserved",
	},
	140: {
		Code:        140,
		Description: "Unrecovered ECC Error",
	},
	141: {
		Code:        141,
		Description: "Reserved",
	},
	142: {
		Code:        142,
		Description: "Unrecovered ECC Error",
	},
	143: {
		Code:        143,
		Description: "GPU Initialization Failure",
	},

	144: {
		Code:        144,
		Description: "NVLINK: SAW Error",
		EventType:   apiv1.EventTypeWarning,
	},
	145: {
		Code:        145,
		Description: "NVLINK: RLW Error",
		EventType:   apiv1.EventTypeWarning,
	},
	146: {
		Code:        146,
		Description: "NVLINK: TLW Error",
		EventType:   apiv1.EventTypeWarning,
	},
	147: {
		Code:        147,
		Description: "NVLINK: TREX Error",
		EventType:   apiv1.EventTypeWarning,
	},
	148: {
		Code:        148,
		Description: "NVLINK: NVLPW_CTRL Error",
		EventType:   apiv1.EventTypeWarning,
	},
	149: {
		Code:        149,
		Description: "NVLINK: NETIR Error",
		EventType:   apiv1.EventTypeWarning,
	},
	150: {
		Code:        150,
		Description: "NVLINK: MSE Error",
		EventType:   apiv1.EventTypeWarning,
	},
	151: {
		Code:        151,
		Description: "Key rotation Error",
	},
	152: {
		Code:        152,
		Description: "DLA SMMU Error",
	},
	153: {
		Code:        153,
		Description: "DLA timeout Error",
	},
	154: {
		Code:        154,
		Description: "GPU Recovery Action Changed",
	},
	155: {
		Code:        155,
		Description: "NVLINK: SW Defined Error",
	},
	156: {
		Code:        156,
		Description: "Resource Retirement Event",
	},
	157: {
		Code:        157,
		Description: "Resource Retirement Failure",
	},
	158: {
		Code:        158,
		Description: "GPU Fatal Timeout",
	},
	159: {
		Code:        159,
		Description: "CHI Non-Data Error",
	},
	160: {
		Code:        160,
		Description: "Channel Retirement Event",
	},
	161: {
		Code:        161,
		Description: "Channel Retirement Failure",
	},
	162: {
		Code:        162,
		Description: "Power Smoothing HW Circuitry capability reengaged",
	},
	163: {
		Code:        163,
		Description: "Power Smoothing HW Circuitry capability disengaged",
	},
	164: {
		Code:        164,
		Description: "Power Smoothing HW Circuitry low lifetime reached",
	},
	165: {
		Code:        165,
		Description: "Power Smoothing HW Circuitry lifetime exhausted",
	},
	166: {
		Code:        166,
		Description: "CC traffic seen prior to link properly being configured for encrypted traffic",
	},
	167: {
		Code:        167,
		Description: "PCIE_FATAL_TIMEOUT",
	},
	168: {
		Code:        168,
		Description: "Errors found in WPR (write protected region)",
	},
	169: {
		Code:        169,
		Description: "Internal micro-controller halt",
	},
	170: {
		Code:        170,
		Description: "Interrupt seen in CC mode",
	},
	171: {
		Code:        171,
		Description: "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
	},
	172: {
		Code:        172,
		Description: "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
	},
	173: {
		Code:        173,
		Description: "C2C Fatal Link Failure",
	},
}

func detailFromNVLinkInfo(info *ExtractedInfo) (*Detail, bool) {
	base, ok := GetDetail(info.Xid)
	if !ok {
		return nil, false
	}

	detailLookup, ok := getDetailWithSubCodeAndStatus(info.Xid, info.SubCode, info.ErrorStatus)
	var detail Detail
	if ok && detailLookup != nil {
		detail = *detailLookup
	} else {
		detail = *base
	}

	if rule, ok := lookupNVLinkRule(info); ok {
		severityEvent := eventTypeFromSeverity(rule.Severity)
		if severityEvent == apiv1.EventTypeUnknown {
			severityEvent = eventTypeFromImmediateBucket(rule.Resolution)
		}
		if severityEvent != apiv1.EventTypeUnknown {
			detail.EventType = severityEvent
		}
		detail.ErrorStatus = rule.ErrorStatus
		// Set the investigatory hint to help differentiate errors with the same Unit.
		// Skip generic values like IGNORE or CONTACT_SUPPORT that don't provide actionable guidance.
		if rule.Investigatory != "" && rule.Investigatory != "IGNORE" && rule.Investigatory != "CONTACT_SUPPORT" {
			detail.InvestigatoryHint = rule.Investigatory
		}
	}

	detail.SubCode = info.SubCode
	detail.SubCodeDescription = info.SubCodeName
	detail.ErrorStatus = info.ErrorStatus
	detail.EventType = maxEventType(detail.EventType, eventTypeFromLogSeverity(info.Severity))
	return &detail, true
}

func buildNVLinkSubCodeDetails() (map[int]map[int]Detail, map[int]map[int]map[uint32]Detail) {
	result := make(map[int]map[int]Detail)
	resultByStatus := make(map[int]map[int]map[uint32]Detail)
	for _, rule := range nvlinkRules {
		if rule.Xid < 144 || rule.Xid > 150 {
			continue
		}
		subCode, ok := subCodeFromRule(rule)
		if !ok {
			continue
		}
		if _, ok := result[rule.Xid]; !ok {
			result[rule.Xid] = make(map[int]Detail)
		}
		if _, ok := resultByStatus[rule.Xid]; !ok {
			resultByStatus[rule.Xid] = make(map[int]map[uint32]Detail)
		}
		if _, ok := resultByStatus[rule.Xid][subCode]; !ok {
			resultByStatus[rule.Xid][subCode] = make(map[uint32]Detail)
		}

		base, ok := GetDetail(rule.Xid)
		if !ok {
			continue
		}
		// Rule-specific detail (preserve rule severity).
		detail := *base
		detail.SubCode = subCode
		detail.SubCodeDescription = rule.Unit
		detail.ErrorStatus = rule.ErrorStatus

		ruleEvent := eventTypeFromSeverity(rule.Severity)
		if ruleEvent == apiv1.EventTypeUnknown {
			ruleEvent = eventTypeFromImmediateBucket(rule.Resolution)
		}
		if ruleEvent != apiv1.EventTypeUnknown {
			detail.EventType = ruleEvent
		}
		// Store rule-specific detail keyed by error status (no aggregation across statuses).
		existing, exists := resultByStatus[rule.Xid][subCode][rule.ErrorStatus]
		if exists {
			detail.EventType = maxEventType(existing.EventType, detail.EventType)
		}
		resultByStatus[rule.Xid][subCode][rule.ErrorStatus] = detail

		// Subcode-level fallback: keep base severity (do not escalate across statuses), but merge actions.
		aggregated, ok := result[rule.Xid][subCode]
		if !ok {
			aggregated = *base
			aggregated.SubCode = subCode
			aggregated.SubCodeDescription = rule.Unit
		}
		result[rule.Xid][subCode] = aggregated
	}

	applyOperationalOverrides(result, resultByStatus)
	return result, resultByStatus
}

func applyOperationalOverrides(result map[int]map[int]Detail, resultByStatus map[int]map[int]map[uint32]Detail) {
	if subMap, ok := result[149]; ok {
		if detail, ok := subMap[4]; ok {
			detail.EventType = apiv1.EventTypeFatal
			detail.SubCodeDescription = "NETIR_LINK_EVT/NETIR_LINK_DOWN (cartridge error)"
			detail.Description = "NVLINK: NETIR Link Event - Possible NVLink cartridge error (contact provider)"
			subMap[4] = detail
			if statusMap, ok := resultByStatus[149][4]; ok {
				for status := range statusMap {
					statusMap[status] = detail
				}
			}
		}
		if detail, ok := subMap[10]; ok {
			detail.EventType = apiv1.EventTypeFatal
			detail.SubCodeDescription = "NETIR_LINK_EVT/NETIR_LINK_DOWN (PHY timeout)"
			detail.Description = "NVLINK: NETIR Link Event - Physical layer retransmission timeout (contact provider)"
			subMap[10] = detail
			if statusMap, ok := resultByStatus[149][10]; ok {
				for status := range statusMap {
					statusMap[status] = detail
				}
			}
		}
	}
}

func indexNVLinkRules() map[int][]nvlinkRule {
	indexed := make(map[int][]nvlinkRule)
	for _, rule := range nvlinkRules {
		indexed[rule.Xid] = append(indexed[rule.Xid], rule)
	}
	return indexed
}

func lookupNVLinkRule(info *ExtractedInfo) (*nvlinkRule, bool) {
	rules := nvlinkRulesByXID[info.Xid]
	for i := range rules {
		rule := &rules[i]
		if !unitMatches(rule.Unit, info.SubCodeName) {
			continue
		}
		if rule.ErrorStatus != info.ErrorStatus {
			continue
		}
		if patternMatches(rule.IntrinfoPatternV2, info.Intrinfo) || patternMatches(rule.IntrinfoPatternV1, info.Intrinfo) {
			return rule, true
		}
	}
	return nil, false
}

func subCodeFromRule(rule nvlinkRule) (int, bool) {
	if value, ok := sampleFromPattern(rule.IntrinfoPatternV2); ok {
		return int((value >> 20) & 0x3F), true
	}
	if value, ok := sampleFromPattern(rule.IntrinfoPatternV1); ok {
		return int((value >> 20) & 0x3F), true
	}
	return 0, false
}

func sampleFromPattern(pattern string) (uint32, bool) {
	if pattern == "" {
		return 0, false
	}
	if len(pattern) != 32 {
		return 0, false
	}
	var value uint32
	for idx, r := range pattern {
		bit := 31 - idx
		switch r {
		case '1':
			value |= 1 << bit
		case '0', '-':
		default:
			return 0, false
		}
	}
	return value, true
}

func patternMatches(pattern string, intrinfo uint32) bool {
	if pattern == "" {
		return true
	}
	if len(pattern) != 32 {
		return false
	}
	for idx, r := range pattern {
		bit := 31 - idx
		switch r {
		case '1':
			if ((intrinfo >> bit) & 1) == 0 {
				return false
			}
		case '0':
			if ((intrinfo >> bit) & 1) == 1 {
				return false
			}
		case '-':
			continue
		default:
			return false
		}
	}
	return true
}

func unitMatches(ruleUnit, logUnit string) bool {
	canonicalLog := normalizeUnit(logUnit)
	if canonicalLog == "" {
		return false
	}
	for _, alias := range unitAliases(ruleUnit) {
		if normalizeUnit(alias) == canonicalLog {
			return true
		}
	}
	return false
}

func unitAliases(ruleUnit string) []string {
	aliases := strings.FieldsFunc(ruleUnit, func(r rune) bool {
		switch r {
		case '/', ',', '(', ')', ' ':
			return true
		default:
			return false
		}
	})
	if len(aliases) == 0 {
		aliases = []string{ruleUnit}
	}
	aliases = append(aliases, ruleUnit)
	return aliases
}

func normalizeUnit(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '_' {
			return r
		}
		return -1
	}, s)
}

// eventTypeFromImmediateBucket maps NVIDIA XID immediate action buckets to event severity types.
// These buckets are defined in NVIDIA's official XID catalog: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
func eventTypeFromImmediateBucket(bucket string) apiv1.EventType {
	switch bucket {
	case "CONTACT_SUPPORT", // Critical GPU error requiring NVIDIA support intervention
		"CHECK_MECHANICALS",    // Hardware connection issues (e.g., auxiliary power not connected, XID 54)
		"WORKFLOW_NVLINK_ERR",  // NVLink hardware error requiring specific diagnostic workflow
		"WORKFLOW_NVLINK5_ERR", // NVLink5-specific error on newer architectures (XIDs 144-150)
		"XID_154",              // GPU Recovery Action Changed - another XID triggered GPU recovery mode change
		"XID_154_EVAL",         // Evaluate if XID 154 should be triggered based on error context (remap errors)
		"RESTART_BM":           // Restart bare metal system/reboot required (e.g., XID 79 - GPU fallen off bus)
		return apiv1.EventTypeFatal
	case "RESET_GPU", // GPU reset required due to timeout, uncontained error, or hung state
		"RESTART_APP",     // Application restart required (copy engine errors, GPU page faults, invalid push buffer)
		"RESTART_VM",      // Virtual machine restart required (e.g., key rotation errors, XID 151)
		"CHECK_UVM",       // Check Unified Virtual Memory subsystem for memory management errors
		"WORKFLOW_XID_48", // XID 48 Double Bit ECC Error workflow - uncorrectable memory errors
		"WORKFLOW_XID_45", // XID 45 Preemptive Removal workflow - cleanup after previous errors (multi-CUDA apps)
		"UPDATE_SWFW":     // Software/firmware update required (e.g., vGPU start errors, XID 78)
		return apiv1.EventTypeCritical
	case "IGNORE", // Non-critical error that can be ignored (e.g., firmware method errors, DLA SMMU errors)
		"": // Empty bucket, typically for unused/deprecated XID codes
		return apiv1.EventTypeInfo
	default:
		return apiv1.EventTypeWarning
	}
}

func eventTypeFromSeverity(severity string) apiv1.EventType {
	switch strings.TrimSpace(strings.ToLower(severity)) {
	case "fatal", "fatal**", "link fatal", "link fatal?":
		return apiv1.EventTypeFatal
	case "non-fatal", "non-fatal*":
		return apiv1.EventTypeWarning
	default:
		return apiv1.EventTypeUnknown
	}
}

func eventTypeFromLogSeverity(severity string) apiv1.EventType {
	switch strings.TrimSpace(strings.ToLower(severity)) {
	case "fatal":
		return apiv1.EventTypeFatal
	case "nonfatal", "non-fatal":
		return apiv1.EventTypeWarning
	default:
		return apiv1.EventTypeUnknown
	}
}

func maxEventType(a, b apiv1.EventType) apiv1.EventType {
	if severityRank(b) > severityRank(a) {
		return b
	}
	return a
}

func severityRank(t apiv1.EventType) int {
	switch t {
	case apiv1.EventTypeInfo:
		return 1
	case apiv1.EventTypeWarning:
		return 2
	case apiv1.EventTypeCritical:
		return 3
	case apiv1.EventTypeFatal:
		return 4
	default:
		return 0
	}
}
