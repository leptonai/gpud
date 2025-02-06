package fd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/fd/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/internal/query"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
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

	// Threshold values for health checks
	CriticalFileHandlesAllocationPercent = 95.0
	WarningFileHandlesAllocationPercent  = 80.0
	CriticalFileDescriptorUsagePercent   = 95.0
	WarningRunningPIDsThresholdPercent   = 80.0

	// Error messages for health checks
	ErrFileHandlesAllocationExceedsCritical = "file handles allocation exceeds 95%"
	ErrFileHandlesAllocationExceedsWarning  = "file handles allocation exceeds its threshold (80%)"
	ErrFileDescriptorUsageExceedsCritical   = "file descriptor usage exceeds 95%"
	ErrRunningPIDsExceedsWarning            = "running PIDs exceeds its threshold (80%)"
	ErrTooManyRunningPIDs                   = "too many running PIDs (exceeds threshold %d)"
	ErrTooManyFileHandlesAllocated          = "too many file handles allocated (exceeds threshold %d)"
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
		Health:  components.StateHealthy,
		Reason: fmt.Sprintf("current file descriptors: %d, threshold: %d, used_percent: %s",
			o.Usage,
			o.ThresholdAllocatedFileHandles,
			o.ThresholdAllocatedFileHandlesPercent,
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

	if thresholdAllocatedPercent, err := o.GetThresholdAllocatedFileHandlesPercent(); err == nil && thresholdAllocatedPercent > WarningFileHandlesAllocationPercent {
		state.Health = components.StateDegraded
		state.Reason += "; " + ErrFileHandlesAllocationExceedsWarning
	}

	// may fail on Mac OS
	if len(o.Errors) > 0 {
		state.Healthy = false
		state.Health = components.StateUnhealthy
		state.Reason += fmt.Sprintf("; %s", strings.Join(o.Errors, ", "))
	}

	return []components.State{state}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			fd_id.Name,
			cfg.Query,
			CreateGet(cfg),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(fd_id.Name)
			} else {
				components_metrics.SetGetSuccess(fd_id.Name)
			}
		}()

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		allocatedFileHandles, _, err := file.GetFileHandles()
		if err != nil {
			return nil, err
		}
		if err := metrics.SetAllocatedFileHandles(ctx, float64(allocatedFileHandles), now); err != nil {
			return nil, err
		}

		runningPIDs, err := process.CountRunningPids()
		if err != nil {
			return nil, err
		}
		if err := metrics.SetRunningPIDs(ctx, float64(runningPIDs), now); err != nil {
			return nil, err
		}

		var errs []string = nil

		// may fail for mac
		// e.g.,
		// stat /proc: no such file or directory
		usage, uerr := file.GetUsage()
		if uerr != nil {
			errs = append(errs, uerr.Error())
		}

		limit, err := file.GetLimit()
		if err != nil {
			return nil, err
		}
		if err := metrics.SetLimit(ctx, float64(limit), now); err != nil {
			return nil, err
		}

		allocatedFileHandlesPct := calcUsagePct(allocatedFileHandles, limit)
		if err := metrics.SetAllocatedFileHandlesPercent(ctx, allocatedFileHandlesPct, now); err != nil {
			return nil, err
		}

		usageVal := runningPIDs // for mac
		if usage > 0 {
			usageVal = usage
		}
		usedPct := calcUsagePct(usageVal, limit)
		if err := metrics.SetUsedPercent(ctx, usedPct, now); err != nil {
			return nil, err
		}

		fileHandlesSupported := file.CheckFileHandlesSupported()
		var thresholdAllocatedFileHandlesPct float64
		if cfg.ThresholdAllocatedFileHandles > 0 {
			thresholdAllocatedFileHandlesPct = calcUsagePct(usage, cfg.ThresholdAllocatedFileHandles)
		}
		if err := metrics.SetThresholdAllocatedFileHandles(ctx, float64(cfg.ThresholdAllocatedFileHandles)); err != nil {
			return nil, err
		}
		if err := metrics.SetThresholdAllocatedFileHandlesPercent(ctx, thresholdAllocatedFileHandlesPct, now); err != nil {
			return nil, err
		}

		fdLimitSupported := file.CheckFDLimitSupported()
		var thresholdRunningPIDsPct float64
		if fdLimitSupported && cfg.ThresholdRunningPIDs > 0 {
			thresholdRunningPIDsPct = calcUsagePct(usage, cfg.ThresholdRunningPIDs)
		}
		if err := metrics.SetThresholdRunningPIDs(ctx, float64(cfg.ThresholdRunningPIDs)); err != nil {
			return nil, err
		}
		if err := metrics.SetThresholdRunningPIDsPercent(ctx, thresholdRunningPIDsPct, now); err != nil {
			return nil, err
		}

		return &Output{
			AllocatedFileHandles: allocatedFileHandles,
			RunningPIDs:          runningPIDs,
			Usage:                usage,
			Limit:                limit,

			AllocatedFileHandlesPercent: fmt.Sprintf("%.2f", allocatedFileHandlesPct),
			UsedPercent:                 fmt.Sprintf("%.2f", usedPct),

			ThresholdAllocatedFileHandles:        cfg.ThresholdAllocatedFileHandles,
			ThresholdAllocatedFileHandlesPercent: fmt.Sprintf("%.2f", thresholdAllocatedFileHandlesPct),

			ThresholdRunningPIDs:        cfg.ThresholdRunningPIDs,
			ThresholdRunningPIDsPercent: fmt.Sprintf("%.2f", thresholdRunningPIDsPct),

			FileHandlesSupported: fileHandlesSupported,
			FDLimitSupported:     fdLimitSupported,

			Errors: errs,
		}, nil
	}
}

func calcUsagePct(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
