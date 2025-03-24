package fd

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/log"
	components_metrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "fd"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	allocatedFileHandles = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles",
			Help:      "tracks the total number of allocated file handles (e.g., /proc/sys/fs/file-nr on Linux)",
		},
	)

	runningPIDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_pids",
			Help:      "tracks the total number of running pids, current file descriptor usage",
		},
	)

	limit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "limit",
			Help:      "tracks the current system-wide file descriptor limit (e.g., /proc/sys/fs/file-max on Linux)",
		},
	)

	allocatedFileHandlesPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles_percent",
			Help:      "tracks the current file descriptor allocation percentage (allocated_file_handles / limit in percentage)",
		},
	)

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage (running_pids / limit in percentage)",
		},
	)

	thresholdRunningPIDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids",
			Help:      "tracks the current file descriptor threshold running pids",
		},
	)
	thresholdRunningPIDsPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids_percent",
			Help:      "tracks the current file descriptor threshold running pids percentage",
		},
	)

	thresholdAllocatedFileHandles = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles",
			Help:      "tracks the current file descriptor threshold allocated file handles",
		},
	)
	thresholdAllocatedFileHandlesPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles_percent",
			Help:      "tracks the current file descriptor threshold allocated percentage",
		},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.allocatedFileHandlesMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_allocated_file_handles")
	c.runningPIDsMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_running_pids")
	c.limitMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_limit")

	c.allocatedFileHandlesPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_allocated_file_handles_percent")
	c.usedPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_percent")

	c.thresholdUsedPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_threshold_used_percent")
	c.thresholdAllocatedFileHandlesPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_threshold_allocated_file_handles_percent")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(allocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(runningPIDs); err != nil {
		return err
	}
	if err := reg.Register(limit); err != nil {
		return err
	}
	if err := reg.Register(allocatedFileHandlesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(thresholdRunningPIDs); err != nil {
		return err
	}
	if err := reg.Register(thresholdRunningPIDsPercent); err != nil {
		return err
	}
	if err := reg.Register(thresholdAllocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(thresholdAllocatedFileHandlesPercent); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	allocatedFileHandles, err := c.readAllocatedFileHandles(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read allocated file handles: %w", err)
	}
	runningPIDs, err := c.readRunningPIDs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read running pids: %w", err)
	}
	limits, err := c.readLimits(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read limits: %w", err)
	}
	allocatedPercents, err := c.readAllocatedFileHandlesPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read allocated percents: %w", err)
	}
	usedPercents, err := c.readUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(allocatedFileHandles)+len(runningPIDs)+len(limits)+len(allocatedPercents)+len(usedPercents))
	for _, m := range allocatedFileHandles {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range runningPIDs {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range limits {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range allocatedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) setLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func (c *component) setAllocatedFileHandles(ctx context.Context, handles float64, currentTime time.Time) error {
	allocatedFileHandles.Set(handles)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.allocatedFileHandlesMetricsStore.Observe(
		ctx,
		handles,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setRunningPIDs(ctx context.Context, pids float64, currentTime time.Time) error {
	runningPIDs.Set(pids)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.runningPIDsMetricsStore.Observe(
		ctx,
		pids,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setLimit(ctx context.Context, v float64, currentTime time.Time) error {
	limit.Set(v)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.limitMetricsStore.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setAllocatedFileHandlesPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	allocatedFileHandlesPercent.Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.allocatedFileHandlesPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}
	return nil
}

func (c *component) setUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	usedPercent.Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.usedPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setThresholdRunningPIDs(ctx context.Context, limit float64) error {
	thresholdRunningPIDs.Set(limit)
	return nil
}

func (c *component) setThresholdRunningPIDsPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	thresholdRunningPIDsPercent.Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.thresholdUsedPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setThresholdAllocatedFileHandles(ctx context.Context, handles float64) error {
	thresholdAllocatedFileHandles.Set(handles)
	return nil
}

func (c *component) setThresholdAllocatedFileHandlesPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	thresholdAllocatedFileHandlesPercent.Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.thresholdAllocatedFileHandlesPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) readAllocatedFileHandles(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.allocatedFileHandlesMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readRunningPIDs(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.runningPIDsMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readLimits(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.limitMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readAllocatedFileHandlesPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.allocatedFileHandlesPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.usedPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readThresholdUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.thresholdUsedPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
