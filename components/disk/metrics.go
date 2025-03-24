package disk

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

const SubSystem = "disk"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	totalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{"mount_point"},
	)

	freeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{"mount_point"},
	)

	usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{"mount_point"},
	)

	usedBytesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent",
			Help:      "tracks the current disk bytes usage percentage",
		},
		[]string{"mount_point"},
	)

	usedInodesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_inodes_percent",
			Help:      "tracks the current disk inodes usage percentage",
		},
		[]string{"mount_point"},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.totalBytesMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_total_bytes")
	c.usedBytesMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_bytes")
	c.usedBytesPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_bytes_percent")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedInodesPercent); err != nil {
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
	usedBytesPercents, err := c.readUsedBytesPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(totalBytes)+len(usedBytes)+len(usedBytesPercents))
	for _, m := range totalBytes {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"mount_point": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range usedBytes {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"mount_point": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range usedBytesPercents {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"mount_point": m.MetricSecondaryName,
			},
		})
	}

	return ms, nil
}

func (c *component) setLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func (c *component) setTotalBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	totalBytes.WithLabelValues(mountPoint).Set(bytes)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.totalBytesMetricsStore.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setFreeBytes(mountPoint string, bytes float64) {
	freeBytes.WithLabelValues(mountPoint).Set(bytes)
}

func (c *component) setUsedBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	usedBytes.WithLabelValues(mountPoint).Set(bytes)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.usedBytesMetricsStore.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setUsedBytesPercent(ctx context.Context, mountPoint string, pct float64, currentTime time.Time) error {
	usedBytesPercent.WithLabelValues(mountPoint).Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.usedBytesPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setUsedInodesPercent(mountPoint string, pct float64) {
	usedInodesPercent.WithLabelValues(mountPoint).Set(pct)
}

func (c *component) readTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.totalBytesMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.usedBytesMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedBytesPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.usedBytesPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
