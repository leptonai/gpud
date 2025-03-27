package metrics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/gpud-metrics/state"
)

// Defines the continuous metrics store interface.
type Store interface {
	// Returns the ID.
	MetricName() string

	// Returns the last value and whether it exists.
	Last(ctx context.Context, opts ...OpOption) (float64, bool, error)

	// Observe the value at the given time and returns the current average.
	// If currentTime is zero, it uses the current system time in UTC.
	Observe(ctx context.Context, value float64, opts ...OpOption) error

	// Returns all the data points since the given time.
	// If since is zero, returns all metrics.
	Read(ctx context.Context, opts ...OpOption) (state.Metrics, error)
}

var _ Store = (*metricsStore)(nil)

type metricsStore struct {
	mu sync.RWMutex

	dbRW *sql.DB
	dbRO *sql.DB

	tableName  string
	metricName string

	secondaryNameToValue map[string]float64
}

// NewStore creates a new persistent averager that stores the data in the database.
func NewStore(dbRW *sql.DB, dbRO *sql.DB, tableName string, metricName string) Store {
	return &metricsStore{
		dbRW:                 dbRW,
		dbRO:                 dbRO,
		tableName:            tableName,
		metricName:           metricName,
		secondaryNameToValue: make(map[string]float64, 1),
	}
}

func (s *metricsStore) MetricName() string {
	return s.metricName
}

func (s *metricsStore) Last(ctx context.Context, opts ...OpOption) (float64, bool, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return 0.0, false, err
	}

	if len(s.secondaryNameToValue) == 0 {
		m, err := state.ReadLastMetric(ctx, s.dbRO, s.tableName, s.metricName, op.metricSecondaryName)
		if err != nil {
			return 0.0, false, err
		}
		if m != nil { // just started with no cache
			s.mu.Lock()
			s.secondaryNameToValue[op.metricSecondaryName] = m.Value
			s.mu.Unlock()
			return m.Value, true, nil
		}
		// no cache, no data (first boot)
	}

	s.mu.RLock()
	v, ok := s.secondaryNameToValue[op.metricSecondaryName]
	s.mu.RUnlock()

	return v, ok, nil
}

func (s *metricsStore) Observe(ctx context.Context, value float64, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	m := state.Metric{
		UnixSeconds:         op.currentTime.Unix(),
		MetricName:          s.metricName,
		MetricSecondaryName: op.metricSecondaryName,
		Value:               value,
	}

	s.mu.Lock()
	s.secondaryNameToValue[op.metricSecondaryName] = value
	s.mu.Unlock()

	return state.InsertMetric(ctx, s.dbRW, s.tableName, m)
}

func (s *metricsStore) Read(ctx context.Context, opts ...OpOption) (state.Metrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}
	return state.ReadMetricsSince(ctx, s.dbRO, s.tableName, s.metricName, op.metricSecondaryName, op.since)
}

type Op struct {
	currentTime         time.Time
	since               time.Time
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

func WithMetricSecondaryName(name string) OpOption {
	return func(op *Op) {
		op.metricSecondaryName = name
	}
}
