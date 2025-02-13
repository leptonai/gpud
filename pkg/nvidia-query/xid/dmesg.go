package xid

import (
	"encoding/json"
	"regexp"
	"strconv"

	query_log "github.com/leptonai/gpud/pkg/query/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
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
	CompiledRegexNVRMXidDmesg      = regexp.MustCompile(RegexNVRMXidDmesg)
	CompiledRegexNVRMXidDeviceUUID = regexp.MustCompile(RegexNVRMXidDeviceUUID)
)

// Extracts the nvidia Xid error code from the dmesg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
func ExtractNVRMXid(line string) int {
	if match := CompiledRegexNVRMXidDmesg.FindStringSubmatch(line); match != nil {
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
	if match := CompiledRegexNVRMXidDeviceUUID.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
}

type DmesgError struct {
	DeviceUUID string         `json:"device_uuid"`
	Detail     *Detail        `json:"detail"`
	LogItem    query_log.Item `json:"log_item"`
}

func (de *DmesgError) JSON() ([]byte, error) {
	return json.Marshal(de)
}

func (de *DmesgError) YAML() ([]byte, error) {
	return yaml.Marshal(de)
}

func ParseDmesgErrorJSON(data []byte) (*DmesgError, error) {
	de := new(DmesgError)
	if err := json.Unmarshal(data, de); err != nil {
		return nil, err
	}
	return de, nil
}

func ParseDmesgErrorYAML(data []byte) (*DmesgError, error) {
	de := new(DmesgError)
	if err := yaml.Unmarshal(data, de); err != nil {
		return nil, err
	}
	return de, nil
}

func ParseDmesgLogLine(time metav1.Time, line string) (DmesgError, error) {
	de := DmesgError{
		DeviceUUID: ExtractNVRMXidDeviceUUID(line),
		LogItem: query_log.Item{
			Line:    line,
			Matched: nil,
			Time:    time,
		},
	}

	errCode := ExtractNVRMXid(line)
	errDetail, ok := GetDetail(errCode)
	if ok {
		de.Detail = errDetail
	}

	return de, nil
}
