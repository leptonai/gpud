package metrics

import (
	"context"
	"time"
)

const (
	// MetricComponentLabelKey is the key for the component of the metric.
	MetricComponentLabelKey = "gpud_component"

	// MetricLabelNamePrefix is the key prefix for the label of the metric.
	// The label key must be prefixed with this key.
	MetricLabelNamePrefix = "label_"
)

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

	// LabelName represents the label key of the metric such as "gpu_uuid", etc..
	LabelName string `json:"label_name,omitempty"`
	// LabelValue represents the label value of the metric such as "GPU-abc", etc..
	LabelValue string `json:"label_value,omitempty"`
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
