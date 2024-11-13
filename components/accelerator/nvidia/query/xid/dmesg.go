package xid

import (
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	query_log "github.com/leptonai/gpud/components/query/log"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"

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
	timestamp, err := pkg_dmesg.ParseCtimeWithError([]byte(line))
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
