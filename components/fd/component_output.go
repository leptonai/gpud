package fd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/components"
)

type Output struct {
	AllocatedFileHandles uint64 `json:"allocated_file_handles"`
	RunningPIDs          uint64 `json:"running_pids"`
	Usage                uint64 `json:"usage"`
	Limit                uint64 `json:"limit"`

	// AllocatedFileHandlesPercent is the percentage of file descriptors that are currently allocated,
	// based on the current file descriptor limit and the current number of file descriptors allocated on the host (not per process).
	AllocatedFileHandlesPercent string `json:"allocated_file_handles_percent"`
	// UsedPercent is the percentage of file descriptors that are currently in use,
	// based on the current file descriptor limit on the host (not per process).
	UsedPercent string `json:"used_percent"`

	ThresholdAllocatedFileHandles        uint64 `json:"threshold_allocated_file_handles"`
	ThresholdAllocatedFileHandlesPercent string `json:"threshold_allocated_file_handles_percent"`

	ThresholdRunningPIDs        uint64 `json:"threshold_running_pids"`
	ThresholdRunningPIDsPercent string `json:"threshold_running_pids_percent"`

	// Set to true if the file handles are supported.
	FileHandlesSupported bool `json:"file_handles_supported"`
	// Set to true if the file descriptor limit is supported.
	FDLimitSupported bool `json:"fd_limit_supported"`

	Errors []string `json:"errors,omitempty"`
}

func (o Output) GetAllocatedFileHandlesPercent() (float64, error) {
	return strconv.ParseFloat(o.AllocatedFileHandlesPercent, 64)
}

func (o Output) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(o.UsedPercent, 64)
}

func (o Output) GetThresholdRunningPIDsPercent() (float64, error) {
	return strconv.ParseFloat(o.ThresholdRunningPIDsPercent, 64)
}

func (o Output) GetThresholdAllocatedFileHandlesPercent() (float64, error) {
	return strconv.ParseFloat(o.ThresholdAllocatedFileHandlesPercent, 64)
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameFileDescriptors = "file_descriptors"

	// The number of file descriptors currently allocated on the host (not per process).
	StateKeyAllocatedFileHandles = "allocated_file_handles"
	// The number of running PIDs returned by https://pkg.go.dev/github.com/shirou/gopsutil/v4/process#Pids.
	StateKeyRunningPIDs = "running_pids"

	StateKeyUsage = "usage"
	StateKeyLimit = "limit"

	StateKeyAllocatedFileHandlesPercent = "allocated_file_handles_percent"
	StateKeyUsedPercent                 = "used_percent"

	StateKeyThresholdAllocatedFileHandles        = "threshold_allocated_file_handles"
	StateKeyThresholdAllocatedFileHandlesPercent = "threshold_allocated_file_handles_percent"
	StateKeyThresholdRunningPIDs                 = "threshold_running_pids"
	StateKeyThresholdRunningPIDsPercent          = "threshold_running_pids_percent"

	// Set to true if the file handles are supported.
	StateKeyFileHandlesSupported = "file_handles_supported"
	// Set to true if the file descriptor limit is supported.
	StateKeyFDLimitSupported = "fd_limit_supported"
)

func ParseStateFileDescriptors(m map[string]string) (*Output, error) {
	o := &Output{}

	var err error
	o.AllocatedFileHandles, err = strconv.ParseUint(m[StateKeyAllocatedFileHandles], 10, 64)
	if err != nil {
		return nil, err
	}
	o.RunningPIDs, err = strconv.ParseUint(m[StateKeyRunningPIDs], 10, 64)
	if err != nil {
		return nil, err
	}
	o.Usage, err = strconv.ParseUint(m[StateKeyUsage], 10, 64)
	if err != nil {
		return nil, err
	}
	o.Limit, err = strconv.ParseUint(m[StateKeyLimit], 10, 64)
	if err != nil {
		return nil, err
	}

	o.AllocatedFileHandlesPercent = m[StateKeyAllocatedFileHandlesPercent]
	o.UsedPercent = m[StateKeyUsedPercent]

	o.ThresholdAllocatedFileHandles, err = strconv.ParseUint(m[StateKeyThresholdAllocatedFileHandles], 10, 64)
	if err != nil {
		return nil, err
	}
	o.ThresholdAllocatedFileHandlesPercent = m[StateKeyThresholdAllocatedFileHandlesPercent]

	o.ThresholdRunningPIDs, err = strconv.ParseUint(m[StateKeyThresholdRunningPIDs], 10, 64)
	if err != nil {
		return nil, err
	}
	o.ThresholdRunningPIDsPercent = m[StateKeyThresholdRunningPIDsPercent]

	o.FileHandlesSupported = m[StateKeyFileHandlesSupported] == "true"
	o.FDLimitSupported = m[StateKeyFDLimitSupported] == "true"

	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameFileDescriptors:
			return ParseStateFileDescriptors(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

func (o *Output) States() ([]components.State, error) {
	state := components.State{
		Name:    StateNameFileDescriptors,
		Healthy: true,
		Reason: fmt.Sprintf("allocated_file_handles: %d, allocated_percent: %s, running_pids: %d, used_percent: %s",
			o.AllocatedFileHandles,
			o.AllocatedFileHandlesPercent,
			o.RunningPIDs,
			o.UsedPercent,
		),
		ExtraInfo: map[string]string{
			StateKeyAllocatedFileHandles: fmt.Sprintf("%d", o.AllocatedFileHandles),
			StateKeyRunningPIDs:          fmt.Sprintf("%d", o.RunningPIDs),
			StateKeyUsage:                fmt.Sprintf("%d", o.Usage),
			StateKeyLimit:                fmt.Sprintf("%d", o.Limit),

			StateKeyAllocatedFileHandlesPercent: o.AllocatedFileHandlesPercent,
			StateKeyUsedPercent:                 o.UsedPercent,

			StateKeyThresholdAllocatedFileHandles:        fmt.Sprintf("%d", o.ThresholdAllocatedFileHandles),
			StateKeyThresholdAllocatedFileHandlesPercent: o.ThresholdAllocatedFileHandlesPercent,

			StateKeyThresholdRunningPIDs:        fmt.Sprintf("%d", o.ThresholdRunningPIDs),
			StateKeyThresholdRunningPIDsPercent: o.ThresholdRunningPIDsPercent,

			StateKeyFileHandlesSupported: fmt.Sprintf("%v", o.FileHandlesSupported),
			StateKeyFDLimitSupported:     fmt.Sprintf("%v", o.FDLimitSupported),
		},
	}

	if allocatedPercent, err := o.GetAllocatedFileHandlesPercent(); err == nil && allocatedPercent > 95.0 {
		state.Healthy = false
		state.Reason += "; allocated_file_handles_percent is greater than 95"
	}
	if thresholdAllocatedPercent, err := o.GetThresholdAllocatedFileHandlesPercent(); err == nil && thresholdAllocatedPercent > 80.0 {
		state.Healthy = false
		state.Reason += "; threshold_allocated_file_handles_percent is greater than 80"
	}

	if usedPercent, err := o.GetUsedPercent(); err == nil && usedPercent > 95.0 {
		state.Healthy = false
		state.Reason += "; used_percent is greater than 95"
	}
	if thresholdRunningPIDsPercent, err := o.GetThresholdRunningPIDsPercent(); err == nil && thresholdRunningPIDsPercent > 80.0 {
		state.Healthy = false
		state.Reason += "; threshold_running_pids_percent is greater than 80"
	}

	if o.FDLimitSupported && o.ThresholdRunningPIDs > 0 && o.RunningPIDs > o.ThresholdRunningPIDs {
		state.Healthy = false
		state.Reason += "; running_pids is greater than threshold_running_pids"
	}
	if o.FileHandlesSupported && o.ThresholdAllocatedFileHandles > 0 && o.AllocatedFileHandles > o.ThresholdAllocatedFileHandles {
		state.Healthy = false
		state.Reason += "; allocated_file_handles is greater than threshold_allocated_file_handles"
	}

	// may fail on Mac OS
	if len(o.Errors) > 0 {
		state.Healthy = false
		state.Reason += fmt.Sprintf("; %s", strings.Join(o.Errors, ", "))
	}

	return []components.State{state}, nil
}
