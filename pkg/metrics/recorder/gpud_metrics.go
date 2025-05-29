package recorder

import (
	"context"
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

var (
	metricFileDescriptorUsage = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "file_descriptor",
			Name:      "usage_total",
			Help:      "total number of file descriptors used",
		},
	)

	metricSQLiteDBSizeInBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_db",
			Name:      "size_bytes",
			Help:      "size of the database in bytes",
		},
	)

	metricSQLiteSelectTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_select",
			Name:      "total",
			Help:      "total number of selects",
		},
	)
	metricSQLiteSelectSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_select",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on selects",
		},
	)

	metricSQLiteInsertUpdateTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_insert_update",
			Name:      "total",
			Help:      "total number of inserts and updates",
		},
	)
	metricSQLiteInsertUpdateSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_insert_update",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on inserts and updates",
		},
	)

	metricSQLiteDeleteTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_delete",
			Name:      "total",
			Help:      "total number of deletes",
		},
	)
	metricSQLiteDeleteSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_delete",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on deletes",
		},
	)

	metricSQLiteVacuumTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_vacuum",
			Name:      "total",
			Help:      "total number of vacuums",
		},
	)
	metricSQLiteVacuumSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpud",
			Subsystem: "sqlite_vacuum",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on vacuums",
		},
	)
)

func init() {
	pkgmetrics.MustRegister(
		metricFileDescriptorUsage,
		metricSQLiteDBSizeInBytes,

		metricSQLiteSelectTotal,
		metricSQLiteSelectSecondsTotal,

		metricSQLiteInsertUpdateTotal,
		metricSQLiteInsertUpdateSecondsTotal,

		metricSQLiteDeleteTotal,
		metricSQLiteDeleteSecondsTotal,

		metricSQLiteVacuumTotal,
		metricSQLiteVacuumSecondsTotal,
	)
}

func recordFileDescriptorUsage(getCurrentProcessUsageFunc func() (uint64, error), metric prometheus.Gauge) error {
	fdUsage, err := getCurrentProcessUsageFunc()
	if err != nil {
		return err
	}
	metric.Set(float64(fdUsage))

	return nil
}

func recordSQLiteDBSize(ctx context.Context, dbRO *sql.DB, metric prometheus.Gauge) error {
	dbSize, err := pkgsqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		return err
	}
	metric.Set(float64(dbSize))

	return nil
}

// RecordSQLiteSelect records the number of selects and the time spent on selects.
func RecordSQLiteSelect(tookSeconds float64) {
	recordSQLiteTotalAndSeconds(tookSeconds, metricSQLiteSelectTotal, metricSQLiteSelectSecondsTotal)
}

// RecordSQLiteInsertUpdate records the number of inserts and updates and the time spent on inserts and updates.
func RecordSQLiteInsertUpdate(tookSeconds float64) {
	recordSQLiteTotalAndSeconds(tookSeconds, metricSQLiteInsertUpdateTotal, metricSQLiteInsertUpdateSecondsTotal)
}

// RecordSQLiteDelete records the number of deletes and the time spent on deletes.
func RecordSQLiteDelete(tookSeconds float64) {
	recordSQLiteTotalAndSeconds(tookSeconds, metricSQLiteDeleteTotal, metricSQLiteDeleteSecondsTotal)
}

// RecordSQLiteVacuum records the number of vacuums and the time spent on vacuums.
func RecordSQLiteVacuum(tookSeconds float64) {
	recordSQLiteTotalAndSeconds(tookSeconds, metricSQLiteVacuumTotal, metricSQLiteVacuumSecondsTotal)
}

func recordSQLiteTotalAndSeconds(tookSeconds float64, totalCounter prometheus.Counter, secondsCounter prometheus.Counter) {
	totalCounter.Inc()
	secondsCounter.Add(tookSeconds)
}
