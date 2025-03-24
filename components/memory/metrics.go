package memory

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

const SubSystem = "memory"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	totalBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total memory in bytes",
		},
	)

	availableBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "available_bytes",
			Help:      "tracks the available memory in bytes",
		},
	)

	usedBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the used memory in bytes",
		},
	)

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of memory used",
		},
	)

	freeBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the free memory in bytes",
		},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.totalBytesMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_total_bytes")
	c.usedBytesMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_bytes")
	c.usedPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_percent")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(availableBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	totalBytes, err := c.readTotalBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}
	usedBytes, err := c.readUsedBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes: %w", err)
	}
	usedPercents, err := c.readUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(totalBytes)+len(usedBytes)+len(usedPercents))
	for _, m := range totalBytes {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedBytes {
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

func (c *component) setTotalBytes(ctx context.Context, bytes float64, currentTime time.Time) error {
	totalBytes.Set(bytes)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.totalBytesMetricsStore.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setAvailableBytes(bytes float64) {
	availableBytes.Set(bytes)
}

func (c *component) setUsedBytes(ctx context.Context, bytes float64, currentTime time.Time) error {
	usedBytes.Set(bytes)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.usedBytesMetricsStore.Observe(
		ctx,
		bytes,
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

	if err := c.usedPercentMetricsStore.Observe(ctx, pct, components_metrics.WithCurrentTime(currentTime)); err != nil {
		return err
	}

	return nil
}

func (c *component) setFreeBytes(bytes float64) {
	freeBytes.Set(bytes)
}

func (c *component) readTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	return c.totalBytesMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	return c.usedBytesMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	return c.usedPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
