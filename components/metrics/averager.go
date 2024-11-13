package metrics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/leptonai/gpud/components/metrics/state"

	_ "github.com/mattn/go-sqlite3"
)

// Defines the continuous averager interface.
type Averager interface {
	// Returns the ID.
	MetricName() string

	// Returns the last value and whether it exists.
	Last(ctx context.Context, opts ...OpOption) (float64, bool, error)

	// Observe the value at the given time and returns the current average.
	// If currentTime is zero, it uses the current system time in UTC.
	Observe(ctx context.Context, value float64, opts ...OpOption) error

	// Avg returns the average value from the "since" time.
	// If since is zero, returns the average value for all data points.
	Avg(ctx context.Context, opts ...OpOption) (float64, error)

	// EMA returns the EMA value from the "since" time.
	// If since is zero, returns the EMA value for all data points.
	// If the ema period is zero, returns the 1-minute EMA value.
	EMA(ctx context.Context, opts ...OpOption) (float64, error)

	// Returns all the data points since the given time.
	// If since is zero, returns all metrics.
	Read(ctx context.Context, opts ...OpOption) (state.Metrics, error)
}

var _ Averager = (*noOpAverager)(nil)

type noOpAverager struct{}

func NewNoOpAverager() Averager {
	return &noOpAverager{}
}

func (n *noOpAverager) MetricName() string {
	return ""
}

func (n *noOpAverager) Last(ctx context.Context, opts ...OpOption) (float64, bool, error) {
	return 0, false, nil
}

func (n *noOpAverager) Observe(ctx context.Context, value float64, opts ...OpOption) error {
	return nil
}

func (n *noOpAverager) Avg(ctx context.Context, opts ...OpOption) (float64, error) {
	return 0, nil
}

func (n *noOpAverager) EMA(ctx context.Context, opts ...OpOption) (float64, error) {
	return 0, nil
}

func (n *noOpAverager) Read(ctx context.Context, opts ...OpOption) (state.Metrics, error) {
	return state.Metrics{}, nil
}

var _ Averager = (*continuousAverager)(nil)

type continuousAverager struct {
	db         *sql.DB
	tableName  string
	metricName string

	secondaryNameToValueMu sync.RWMutex
	secondaryNameToValue   map[string]float64
}

func NewAverager(db *sql.DB, tableName string, metricName string) Averager {
	return &continuousAverager{
		db:                   db,
		tableName:            tableName,
		metricName:           metricName,
		secondaryNameToValue: make(map[string]float64, 1),
	}
}

func (c *continuousAverager) MetricName() string {
	return c.metricName
}

func (c *continuousAverager) Last(ctx context.Context, opts ...OpOption) (float64, bool, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return 0.0, false, err
	}

	if len(c.secondaryNameToValue) == 0 {
		m, err := state.ReadLastMetric(ctx, c.db, c.tableName, c.metricName, op.metricSecondaryName)
		if err != nil {
			return 0.0, false, err
		}
		if m != nil { // just started with no cache
			c.secondaryNameToValueMu.Lock()
			c.secondaryNameToValue[op.metricSecondaryName] = m.Value
			c.secondaryNameToValueMu.Unlock()
			return m.Value, true, nil
		}
		// no cache, no data (first boot)
	}

	c.secondaryNameToValueMu.RLock()
	v, ok := c.secondaryNameToValue[op.metricSecondaryName]
	c.secondaryNameToValueMu.RUnlock()

	return v, ok, nil
}

func (c *continuousAverager) Observe(ctx context.Context, value float64, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	m := state.Metric{
		UnixSeconds:         op.currentTime.Unix(),
		MetricName:          c.metricName,
		MetricSecondaryName: op.metricSecondaryName,
		Value:               value,
	}

	c.secondaryNameToValueMu.Lock()
	c.secondaryNameToValue[op.metricSecondaryName] = value
	c.secondaryNameToValueMu.Unlock()

	return state.InsertMetric(ctx, c.db, c.tableName, m)
}

// Avg returns the average value from the "since" time.
// If since is zero, returns the average value for all data points.
func (c *continuousAverager) Avg(ctx context.Context, opts ...OpOption) (float64, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return 0.0, err
	}
	return state.AvgSince(ctx, c.db, c.tableName, c.metricName, op.metricSecondaryName, op.since)
}

// EMA returns the EMA value from the "since" time.
// If since is zero, returns the EMA value for all data points.
func (c *continuousAverager) EMA(ctx context.Context, opts ...OpOption) (float64, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return 0.0, err
	}
	return state.EMASince(ctx, c.db, c.tableName, c.metricName, op.metricSecondaryName, op.emaPeriod, op.since)
}

func (c *continuousAverager) Read(ctx context.Context, opts ...OpOption) (state.Metrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}
	return state.ReadMetricsSince(ctx, c.db, c.tableName, c.metricName, op.metricSecondaryName, op.since)
}

type Op struct {
	currentTime         time.Time
	since               time.Time
	emaPeriod           time.Duration
	metricSecondaryName string
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.currentTime.IsZero() {
		op.currentTime = time.Now().UTC()
	}
	if op.emaPeriod == 0 {
		op.emaPeriod = time.Minute
	}

	return nil
}

func WithCurrentTime(t time.Time) OpOption {
	return func(op *Op) {
		op.currentTime = t
	}
}

func WithSince(t time.Time) OpOption {
	return func(op *Op) {
		op.since = t
	}
}

func WithEMAPeriod(period time.Duration) OpOption {
	return func(op *Op) {
		op.emaPeriod = period
	}
}

func WithMetricSecondaryName(name string) OpOption {
	return func(op *Op) {
		op.metricSecondaryName = name
	}
}
