package fd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/fd/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"
)

type Output struct {
	RunningPIDs uint64   `json:"running_pids"`
	Usage       uint64   `json:"usage"`
	Limit       uint64   `json:"limit"`
	UsedPercent string   `json:"used_percent"`
	Errors      []string `json:"errors,omitempty"`
}

func (o Output) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(o.UsedPercent, 64)
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
	StateKeyRunningPIDs = "running_pids"
	StateKeyUsage       = "usage"
	StateKeyLimit       = "limit"
	StateKeyUsedPercent = "used_percent"
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
		Reason:  fmt.Sprintf("running_pids: %d, usage: %d, limit: %d, used_percent: %s", o.RunningPIDs, o.Usage, o.Limit, o.UsedPercent),
		ExtraInfo: map[string]string{
			StateKeyRunningPIDs: fmt.Sprintf("%d", o.RunningPIDs),
			StateKeyUsage:       fmt.Sprintf("%d", o.Usage),
			StateKeyLimit:       fmt.Sprintf("%d", o.Limit),
			StateKeyUsedPercent: o.UsedPercent,
		},
	}
	if usedPercent, err := o.GetUsedPercent(); err == nil && usedPercent > 95.0 {
		state.Healthy = false
		state.Reason += "-- used_percent is greater than 95"
	}

	// may fail on Mac OS
	if len(o.Errors) > 0 {
		state.Healthy = false
		state.Reason += fmt.Sprintf("-- %s", strings.Join(o.Errors, ", "))
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
		defaultPoller = query.New(Name, cfg.Query, Get)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(Name)
		} else {
			components_metrics.SetGetSuccess(Name)
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
	usage, uerr := getUsage()
	if uerr != nil {
		errs = append(errs, uerr.Error())
	}

	limit, err := getLimit()
	if err != nil {
		return nil, err
	}
	if err := metrics.SetLimit(ctx, float64(limit), now); err != nil {
		return nil, err
	}

	var usedPct float64
	if limit > 0 {
		usageVal := runningPIDs // for mac
		if usage > 0 {
			usageVal = usage
		}
		usedPct = float64(usageVal) / float64(limit) * 100
	}
	if err := metrics.SetUsedPercent(ctx, usedPct, now); err != nil {
		return nil, err
	}

	return &Output{
		RunningPIDs: runningPIDs,
		Usage:       usage,
		Limit:       limit,
		UsedPercent: fmt.Sprintf("%.2f", usedPct),
		Errors:      errs,
	}, nil
}

func getRunningPids() (uint64, error) {
	pids, err := process.Pids()
	if err != nil {
		return 0, err
	}
	return uint64(len(pids)), nil
}

// "process_open_fds" in prometheus collector
// ref. https://github.com/prometheus/client_golang/blob/main/prometheus/process_collector_other.go
// ref. https://pkg.go.dev/github.com/prometheus/procfs
func getUsage() (uint64, error) {
	procs, err := procfs.AllProcs()
	if err != nil {
		return 0, err
	}
	total := uint64(0)
	for _, proc := range procs {
		l, err := proc.FileDescriptorsLen()
		if err != nil {
			// If the error is due to the file descriptor being cleaned up and not used anymore,
			// skip to the next process ID.
			if os.IsNotExist(err) ||
				strings.Contains(err.Error(), "no such file or directory") ||

				// e.g., stat /proc/1321147/fd: no such process
				strings.Contains(err.Error(), "no such process") {
				continue
			}

			return 0, err
		}
		total += uint64(l)
	}
	return total, nil
}

func getLimit() (uint64, error) {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return 0, err
	}
	return rlimit.Cur, nil
}
