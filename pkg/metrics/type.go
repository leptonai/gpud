package metrics

import (
	"context"
	"time"
)

const (
	// MetricComponentLabelKey is the key for the component of the metric.
	MetricComponentLabelKey = "gpud_component"

	// MetricLabelKey is the key for the label of the metric.
	MetricLabelKey = "gpud_metric_label"
)

// Metric represents a metric row in the database table.
type Metric struct {
	// UnixMilliseconds represents the Unix timestamp of the metric.
	UnixMilliseconds int64 `json:"unix_milliseconds"`
	// Component represents the name of the component this metric belongs to.
	Component string `json:"component"`
	// Name represents the name of the metric.
	Name string `json:"name"`
	// Label represents the label of the metric such as GPU ID, etc..
	Label string `json:"label,omitempty"`
	// Value represents the numeric value of the metric.
	Value float64 `json:"value"`
}

// Metrics is a slice of Metric.
type Metrics []Metric

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
