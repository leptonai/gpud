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
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/pkg/file"

	"github.com/shirou/gopsutil/v4/process"
)

type Output struct {
	RunningPIDs uint64 `json:"running_pids"`
	Usage       uint64 `json:"usage"`

	Limit uint64 `json:"limit"`
	// UsedPercent is the percentage of file descriptors that are currently in use,
	// based on the current file descriptor limit on the host (not per process).
	UsedPercent string `json:"used_percent"`

	// Set to true if the max file descriptor is supported (e.g., /proc/sys/fs/file-max exists on linux).
	FDLimitSupported bool `json:"fd_limit_supported"`

	ThresholdLimit uint64 `json:"threshold_limit"`
	// ThresholdUsedPercent is the percentage of file descriptors that are currently in use,
	// based on the threshold file descriptor limit.
	ThresholdUsedPercent string `json:"threshold_used_percent"`

	Errors []string `json:"errors,omitempty"`
}

func (o Output) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(o.UsedPercent, 64)
}

func (o Output) GetThresholdUsedPercent() (float64, error) {
	return strconv.ParseFloat(o.ThresholdUsedPercent, 64)
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

	// The number of running PIDs returned by https://pkg.go.dev/github.com/shirou/gopsutil/v4/process#Pids.
	StateKeyRunningPIDs          = "running_pids"
	StateKeyUsage                = "usage"
	StateKeyLimit                = "limit"
	StateKeyUsedPercent          = "used_percent"
	StateKeyFDLimitSupported     = "fd_limit_supported"
	StateKeyThresholdLimit       = "threshold_limit"
	StateKeyThresholdUsedPercent = "threshold_used_percent"
)

func ParseStateFileDescriptors(m map[string]string) (*Output, error) {
	o := &Output{}

	var err error
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
	o.UsedPercent = m[StateKeyUsedPercent]

	o.ThresholdLimit, err = strconv.ParseUint(m[StateKeyThresholdLimit], 10, 64)
	if err != nil {
		return nil, err
	}
	o.ThresholdUsedPercent = m[StateKeyThresholdUsedPercent]

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
		Reason:  fmt.Sprintf("running_pids: %d, usage: %d, limit: %d, threshold_limit: %d, used_percent: %s, threshold_used_percent: %s", o.RunningPIDs, o.Usage, o.Limit, o.ThresholdLimit, o.UsedPercent, o.ThresholdUsedPercent),
		ExtraInfo: map[string]string{
			StateKeyRunningPIDs: fmt.Sprintf("%d", o.RunningPIDs),
			StateKeyUsage:       fmt.Sprintf("%d", o.Usage),

			StateKeyLimit:       fmt.Sprintf("%d", o.Limit),
			StateKeyUsedPercent: o.UsedPercent,

			StateKeyFDLimitSupported: fmt.Sprintf("%v", o.FDLimitSupported),

			StateKeyThresholdLimit:       fmt.Sprintf("%d", o.ThresholdLimit),
			StateKeyThresholdUsedPercent: o.ThresholdUsedPercent,
		},
	}

	if usedPercent, err := o.GetUsedPercent(); err == nil && usedPercent > 95.0 {
		state.Healthy = false
		state.Reason += "-- used_percent is greater than 95"
	}

	if o.FDLimitSupported && o.ThresholdLimit > 0 {
		if thresholdUsedPercent, err := o.GetThresholdUsedPercent(); err == nil && thresholdUsedPercent > 95.0 {
			state.Healthy = false
			state.Reason += "-- threshold_used_percent is greater than 95"
		}
	}

	// may fail on Mac OS
	if len(o.Errors) > 0 {
		state.Healthy = false
		state.Reason += fmt.Sprintf(" -- %s", strings.Join(o.Errors, ", "))
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
		defaultPoller = query.New(fd_id.Name, cfg.Query, CreateGet(cfg))
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

		runningPIDs, err := getRunningPids()
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

		usageVal := runningPIDs // for mac
		if usage > 0 {
			usageVal = usage
		}
		usedPct := calculateUsedPercent(usageVal, limit)
		if err := metrics.SetUsedPercent(ctx, usedPct, now); err != nil {
			return nil, err
		}

		fdLimitSupported := file.CheckFDLimitSupported()

		var thresholdUsedPct float64
		if fdLimitSupported && cfg.ThresholdLimit > 0 {
			thresholdUsedPct = calculateUsedPercent(usage, cfg.ThresholdLimit)
		}
		if err := metrics.SetThresholdLimit(ctx, float64(cfg.ThresholdLimit)); err != nil {
			return nil, err
		}
		if err := metrics.SetThresholdUsedPercent(ctx, thresholdUsedPct, now); err != nil {
			return nil, err
		}

		return &Output{
			RunningPIDs: runningPIDs,
			Usage:       usage,

			Limit:       limit,
			UsedPercent: fmt.Sprintf("%.2f", usedPct),

			FDLimitSupported: fdLimitSupported,

			ThresholdLimit:       cfg.ThresholdLimit,
			ThresholdUsedPercent: fmt.Sprintf("%.2f", thresholdUsedPct),

			Errors: errs,
		}, nil
	}
}

func getRunningPids() (uint64, error) {
	pids, err := process.Pids()
	if err != nil {
		return 0, err
	}
	return uint64(len(pids)), nil
}

func calculateUsedPercent(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
