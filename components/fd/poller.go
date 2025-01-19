package fd

import (
	"context"
	"fmt"
	"sync"
	"time"

	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/fd/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/poller"
	"github.com/leptonai/gpud/pkg/process"
)

var (
	defaultPollerOnce sync.Once
	defaultPoller     poller.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = poller.New(
			fd_id.Name,
			cfg.PollerConfig,
			createGet(cfg),
			nil,
		)
	})
}

func getDefaultPoller() poller.Poller {
	return defaultPoller
}

func createGet(cfg Config) poller.GetFunc {
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
		if fileHandlesSupported && cfg.ThresholdAllocatedFileHandles > 0 {
			thresholdAllocatedFileHandlesPct = calcUsagePct(allocatedFileHandles, cfg.ThresholdAllocatedFileHandles)
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
