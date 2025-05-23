package metrics

import (
	"context"
	"time"
)

// MetricComponentLabelKey is the key for the component of the metric.
const MetricComponentLabelKey = "gpud_component"

// Metric represents a metric row in the database table.
type Metric struct {
	// UnixMilliseconds represents the Unix timestamp of the metric.
	UnixMilliseconds int64 `json:"unix_milliseconds"`
	// Component represents the name of the component this metric belongs to.
	Component string `json:"component"`
	// Name represents the name of the metric.
	Name string `json:"name"`
	// Value represents the numeric value of the metric.
	Value float64 `json:"value"`

	// Labels represents all the labels of the metric.
	Labels map[string]string `json:"labels,omitempty"`
}

// Metrics is a slice of Metric.
type Metrics []Metric

// Recorder defines the metrics recorder interface.
type Recorder interface {
	// Start starts the periodic metrics recorder.
	Start()
}

// Scraper defines the metrics scraper interface.
type Scraper interface {
	Scrape(context.Context) (Metrics, error)
}

// Store defines the metrics store interface.
type Store interface {
	// Record records metric data points.
	Record(ctx context.Context, ms ...Metric) error

	// Returns all the data points since the given time.
	// If since is zero, returns all metrics.
	Read(ctx context.Context, opts ...OpOption) (Metrics, error)

	// Purge purges the metrics data points before the given time.
	Purge(ctx context.Context, before time.Time) (int, error)
}
