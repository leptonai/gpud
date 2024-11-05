package xid

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	query_log "github.com/leptonai/gpud/components/query/log"

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
)

var CompiledRegexNVRMXidDmesg = regexp.MustCompile(RegexNVRMXidDmesg)

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

type DmesgError struct {
	Detail  *Detail        `json:"detail"`
	LogItem query_log.Item `json:"log_item"`
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

func ParseDmesgLogLine(line string) (DmesgError, error) {
	timestamp, err := parseDmesgLine(line)
	if err != nil {
		timestamp = time.Now()
	}
	de := DmesgError{
		LogItem: query_log.Item{
			Line:    line,
			Matched: nil,
			Time:    metav1.Time{Time: timestamp.UTC()},
		},
	}

	errCode := ExtractNVRMXid(line)
	errDetail, ok := GetDetail(errCode)
	if ok {
		de.Detail = errDetail
	}

	return de, nil
}

// parseDmesgLine parses a single line from dmesg -T output
// Example line: [Thu Aug  8 11:50:58 2024] IPv6: ADDRCONF(NETDEV_CHANGE): calic8a3d4799be: link becomes ready
func parseDmesgLine(line string) (time.Time, error) {
	r := regexp.MustCompile(`^\[(.*)\]`)
	matches := r.FindStringSubmatch(line)
	if len(matches) < 1 {
		return time.Time{}, fmt.Errorf("failed to parse line: %s", line)
	}
	timeStr := matches[1]
	const timeLayout = "Mon Jan 2 15:04:05 2006"
	parsedTime, err := time.Parse(timeLayout, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time: %s", timeStr)
	}
	return parsedTime, nil
}
