package sqlite

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

var (
	metricInsertUpdateTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "insert_update",
			Name:      "total",
			Help:      "total number of inserts and updates",
		},
	)
	metricInsertUpdateSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "insert_update",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on inserts and updates",
		},
	)

	metricDeleteTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "delete",
			Name:      "total",
			Help:      "total number of deletes",
		},
	)
	metricDeleteSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "delete",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on deletes",
		},
	)

	metricSelectTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "select",
			Name:      "total",
			Help:      "total number of selects",
		},
	)
	metricSelectSecondsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "sqlite",
			Subsystem: "select",
			Name:      "seconds_total",
			Help:      "total number of seconds spent on selects",
		},
	)
)

func init() {
	pkgmetrics.MustRegister(
		metricInsertUpdateTotal,
		metricInsertUpdateSecondsTotal,
		metricDeleteTotal,
		metricDeleteSecondsTotal,
		metricSelectTotal,
		metricSelectSecondsTotal,
	)
}

func RecordInsertUpdate(tookSeconds float64) {
	metricInsertUpdateTotal.Inc()
	metricInsertUpdateSecondsTotal.Add(tookSeconds)
}

func RecordDelete(tookSeconds float64) {
	metricDeleteTotal.Inc()
	metricDeleteSecondsTotal.Add(tookSeconds)
}

func RecordSelect(tookSeconds float64) {
	metricSelectTotal.Inc()
	metricSelectSecondsTotal.Add(tookSeconds)
}

type Metrics struct {
	Time time.Time

	// The total number of inserts and updates in cumulative count.
	InsertUpdateTotal        int64
	InsertUpdateSecondsTotal float64
	InsertUpdateSecondsAvg   float64

	// The total number of deletes in cumulative count.
	DeleteTotal        int64
	DeleteSecondsTotal float64
	DeleteSecondsAvg   float64

	// The total number of selects in cumulative count.
	SelectTotal        int64
	SelectSecondsTotal float64
	SelectSecondsAvg   float64
}

func (m Metrics) IsZero() bool {
	return m.InsertUpdateTotal == 0 &&
		m.InsertUpdateSecondsTotal == 0 &&
		m.InsertUpdateSecondsAvg == 0 &&
		m.DeleteTotal == 0 &&
		m.DeleteSecondsTotal == 0 &&
		m.DeleteSecondsAvg == 0 &&
		m.SelectTotal == 0 &&
		m.SelectSecondsTotal == 0 &&
		m.SelectSecondsAvg == 0
}

func ReadMetrics(gatherer prometheus.Gatherer) (Metrics, error) {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		return Metrics{}, err
	}

	mtr := Metrics{
		Time: time.Now().UTC(),
	}
	for _, mf := range metricFamilies {
		metricName := mf.GetName()

		if metricName == "sqlite_insert_update_total" {
			for _, m := range mf.GetMetric() {
				mtr.InsertUpdateTotal = int64(m.GetCounter().GetValue())
			}
		}
		if metricName == "sqlite_insert_update_seconds_total" {
			for _, m := range mf.GetMetric() {
				mtr.InsertUpdateSecondsTotal = m.GetCounter().GetValue()
			}
		}

		if metricName == "sqlite_delete_total" {
			for _, m := range mf.GetMetric() {
				mtr.DeleteTotal = int64(m.GetCounter().GetValue())
			}
		}
		if metricName == "sqlite_delete_seconds_total" {
			for _, m := range mf.GetMetric() {
				mtr.DeleteSecondsTotal = m.GetCounter().GetValue()
			}
		}

		if metricName == "sqlite_select_total" {
			for _, m := range mf.GetMetric() {
				mtr.SelectTotal = int64(m.GetCounter().GetValue())
			}
		}
		if metricName == "sqlite_select_seconds_total" {
			for _, m := range mf.GetMetric() {
				mtr.SelectSecondsTotal = m.GetCounter().GetValue()
			}
		}
	}

	if mtr.InsertUpdateTotal > 0 {
		mtr.InsertUpdateSecondsAvg = mtr.InsertUpdateSecondsTotal / float64(mtr.InsertUpdateTotal)
	}
	if mtr.DeleteTotal > 0 {
		mtr.DeleteSecondsAvg = mtr.DeleteSecondsTotal / float64(mtr.DeleteTotal)
	}
	if mtr.SelectTotal > 0 {
		mtr.SelectSecondsAvg = mtr.SelectSecondsTotal / float64(mtr.SelectTotal)
	}

	return mtr, nil
}

// Computes the QPS for insert/updates, deletes, and selects, based on the previous and current metrics time.
func (prev Metrics) QPS(cur Metrics) (insertUpdateAvgQPS float64, deleteAvgQPS float64, selectAvgQPS float64) {
	insertUpdateAvgQPS = float64(0)
	deleteAvgQPS = float64(0)
	selectAvgQPS = float64(0)

	elapsedSeconds := cur.Time.Sub(prev.Time).Seconds()
	if !prev.IsZero() && !cur.IsZero() && elapsedSeconds > 0 {
		insertUpdateAvgQPS = float64(cur.InsertUpdateTotal-prev.InsertUpdateTotal) / elapsedSeconds
		deleteAvgQPS = float64(cur.DeleteTotal-prev.DeleteTotal) / elapsedSeconds
		selectAvgQPS = float64(cur.SelectTotal-prev.SelectTotal) / elapsedSeconds
	}

	return insertUpdateAvgQPS, deleteAvgQPS, selectAvgQPS
}
