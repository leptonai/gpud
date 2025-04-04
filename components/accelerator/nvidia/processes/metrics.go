package processes

import (
	"context"
	"database/sql"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_processes"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-processes",
	}

	metricRunningProcesses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_total",
			Help:      "tracks the current per-GPU process counter",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(metricRunningProcesses); err != nil {
		return err
	}

	return nil
}

// TO BE DEPRECATED
func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}
