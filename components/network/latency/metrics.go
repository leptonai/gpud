package latency

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "network_latency"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "network-latency",
	}

	edgeInMilliseconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "edge_in_milliseconds",
			Help:      "tracks the edge latency in milliseconds",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is provider region
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = &component{}

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(edgeInMilliseconds); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}
