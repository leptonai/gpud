package sxid

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
	// [111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)
	// [131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up
	//
	// ref.
	// "D.4 Non-Fatal NVSwitch SXid Errors"
	// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
	RegexNVSwitchSXidDmesg = `SXid.*?: (\d+),`
)

var CompiledRegexNVSwitchSXidDmesg = regexp.MustCompile(RegexNVSwitchSXidDmesg)

// Extracts the nvidia NVSwitch SXid error code from the dmesg log line.
// Returns 0 if the error code is not found.
// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
func ExtractNVSwitchSXid(line string) int {
	if match := CompiledRegexNVSwitchSXidDmesg.FindStringSubmatch(line); match != nil {
		if id, err := strconv.Atoi(match[1]); err == nil {
			return id
		}
	}
	return 0
}

type DmesgError struct {
	Detail  *Detail        `json:"detail,omitempty"`
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

	errCode := ExtractNVSwitchSXid(line)
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
