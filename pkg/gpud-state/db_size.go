package gpudstate

import (
	"context"
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
)

var (
	metricCurrentSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "state_sqlite",
			Name:      "current_size",
			Help:      "current size of the database file (number of pages * size of page)",
		},
	)
)

func init() {
	pkgmetrics.MustRegister(metricCurrentSize)
}

func RecordDBSize(ctx context.Context, db *sql.DB) error {
	dbSize, err := sqlite.ReadDBSize(ctx, db)
	if err != nil {
		return err
	}
	metricCurrentSize.Set(float64(dbSize))

	return nil
}
