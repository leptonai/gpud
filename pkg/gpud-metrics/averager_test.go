package gpudmetrics

import (
	"context"
	"testing"
	"time"

	metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewAverager(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	a := NewAverager(dbRW, dbRO, "test_table", "test_name")
	if a == nil {
		t.Fatal("NewAverager returned nil")
	}
}

func TestAveragerObserve(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	a := NewAverager(dbRW, dbRO, "test_table", "test_name")
	now := time.Now()

	numPoints := 500
	values := make([]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		values[i] = float64(i + 1)
		err := a.Observe(ctx, values[i], WithCurrentTime(now))
		if err != nil {
			t.Errorf("Observe(%f) returned error: %v", values[i], err)
		}
		now = now.Add(time.Second)

		last, ok, err := a.Last(ctx)
		if err != nil {
			t.Errorf("Last() returned error: %v", err)
		}
		if !ok {
			t.Errorf("Last() returned ok = false")
		}
		if last != values[i] {
			t.Errorf("Last() = %f; want %f", last, values[i])
		}
	}

	start := time.Now()
	avgResult, err := a.Avg(ctx)
	latency := time.Since(start)
	t.Logf("AvgSince latency: %v", latency)
	if err != nil {
		t.Errorf("AvgSince() returned error: %v", err)
	}

	// test EMA
	start = time.Now()
	emaResult, err := a.EMA(ctx, WithEMAPeriod(time.Minute))
	emaLatency := time.Since(start)
	t.Logf("EMA latency: %v", emaLatency)
	if err != nil {
		t.Errorf("EMA() returned error: %v", err)
	}

	// EMA should be closer to recent values, so it should be higher than the average
	if emaResult <= avgResult {
		t.Errorf("EMA() = %f; expected to be greater than AvgSince() = %f", emaResult, avgResult)
	}

	t.Logf("AvgSince result: %f", avgResult)
	t.Logf("EMA result: %f", emaResult)

	expectedAvg := 250.5 // (1 + 500) / 2
	if avgResult != expectedAvg {
		t.Errorf("AvgSince() = %f; want %f", avgResult, expectedAvg)
	}

	// test EMA with different time ranges
	testRanges := []struct {
		name     string
		duration time.Duration
	}{
		{"Last 1 minute", time.Minute},
		{"Last 5 minutes", 5 * time.Minute},
		{"Last 10 minutes", 10 * time.Minute},
	}

	for _, tr := range testRanges {
		since := now.Add(-tr.duration)
		emaResult, err := a.EMA(ctx, WithEMAPeriod(time.Minute), WithSince(since))
		if err != nil {
			t.Errorf("EMA() for %s returned error: %v", tr.name, err)
		}
		avgResult, err := a.Avg(ctx, WithSince(since))
		if err != nil {
			t.Errorf("AvgSince() for %s returned error: %v", tr.name, err)
		}
		t.Logf("%s - AvgSince: %f, EMA: %f", tr.name, avgResult, emaResult)
	}

	allMetrics, err := a.Read(ctx)
	if err != nil {
		t.Errorf("All() returned error: %v", err)
	}
	if len(allMetrics) != numPoints {
		t.Errorf("All() returned %d metrics; want %d", len(allMetrics), numPoints)
	}

	for i, metric := range allMetrics {
		expectedValue := float64(i + 1)
		if metric.Value != expectedValue {
			t.Errorf("All()[%d].Value = %f; want %f", i, metric.Value, expectedValue)
		}
	}

	// test All with a specific time range
	halfwayPoint := now.Add(-time.Duration(numPoints/2) * time.Second)
	halfMetrics, err := a.Read(ctx, WithSince(halfwayPoint))
	if err != nil {
		t.Errorf("All() with halfway point returned error: %v", err)
	}
	expectedHalfLength := numPoints / 2
	if len(halfMetrics) != expectedHalfLength {
		t.Errorf("All() with halfway point returned %d metrics; want %d", len(halfMetrics), expectedHalfLength)
	}

	// check if the correct half of metrics are present
	for i, metric := range halfMetrics {
		expectedValue := float64(i + 1 + numPoints/2)
		if metric.Value != expectedValue {
			t.Errorf("All() with halfway point[%d].Value = %f; want %f", i, metric.Value, expectedValue)
		}
	}
}

func TestAveragerAll(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	a := NewAverager(dbRW, dbRO, "test_table", "test_name")
	now := time.Now()

	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	for _, v := range values {
		err := a.Observe(ctx, v, WithCurrentTime(now))
		if err != nil {
			t.Errorf("Observe(%f) returned error: %v", v, err)
		}
		now = now.Add(time.Second)
	}

	tests := []struct {
		name     string
		since    time.Time
		expected []float64
	}{
		{"All metrics", time.Time{}, []float64{1.0, 2.0, 3.0, 4.0, 5.0}},
		{"Last 3 metrics", now.Add(-3 * time.Second), []float64{3.0, 4.0, 5.0}},
		{"Future time", now.Add(time.Hour), nil},
		{"No metrics", now.Add(-time.Hour), []float64{1.0, 2.0, 3.0, 4.0, 5.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.Read(ctx, WithSince(tt.since))
			if err != nil {
				t.Errorf("All(%v) returned error: %v", tt.since, err)
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("All(%v) returned %d metrics; want %d", tt.since, len(result), len(tt.expected))
			}
			for i, v := range result {
				if v.Value != tt.expected[i] {
					t.Errorf("All(%v)[%d] = %f; want %f", tt.since, i, v.Value, tt.expected[i])
				}
			}
		})
	}
}

func TestEmptyAverager(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	a := NewAverager(dbRW, dbRO, "test_table", "test_name")

	result, err := a.Avg(ctx)
	if err != nil {
		t.Errorf("Read() returned error: %v", err)
	}
	if result != 0.0 {
		t.Errorf("Read() on empty averager = %f; want 0.0", result)
	}

	if _, err = a.Read(ctx); err != nil {
		t.Errorf("All() on empty averager should return nil")
	}
}

func TestContinuousAveragerRead(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	createTime := func(minutes int) time.Time {
		return time.Date(2025, 1, 1, 0, minutes, 0, 0, time.UTC)
	}

	tests := []struct {
		name     string
		setup    func() *continuousAverager
		since    time.Time
		expected float64
	}{
		{
			name: "empty averager",
			setup: func() *continuousAverager {
				return NewAverager(dbRW, dbRO, "test_table", "empty averager").(*continuousAverager)
			},
			since:    time.Time{},
			expected: 0.0,
		},
		{
			name: "all values",
			setup: func() *continuousAverager {
				a := NewAverager(dbRW, dbRO, "test_table", "all values").(*continuousAverager)
				if err := a.Observe(ctx, 1.0, WithCurrentTime(createTime(1))); err != nil {
					t.Fatalf("Observe(1.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 2.0, WithCurrentTime(createTime(2))); err != nil {
					t.Fatalf("Observe(2.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 3.0, WithCurrentTime(createTime(3))); err != nil {
					t.Fatalf("Observe(3.0) returned error: %v", err)
				}
				return a
			},
			since:    time.Time{},
			expected: 2.0,
		},
		{
			name: "since middle",
			setup: func() *continuousAverager {
				a := NewAverager(dbRW, dbRO, "test_table", "since middle").(*continuousAverager)
				if err := a.Observe(ctx, 1.0, WithCurrentTime(createTime(1))); err != nil {
					t.Fatalf("Observe(1.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 2.0, WithCurrentTime(createTime(2))); err != nil {
					t.Fatalf("Observe(2.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 3.0, WithCurrentTime(createTime(3))); err != nil {
					t.Fatalf("Observe(3.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 4.0, WithCurrentTime(createTime(4))); err != nil {
					t.Fatalf("Observe(4.0) returned error: %v", err)
				}
				return a
			},
			since:    createTime(2),
			expected: 3.0,
		},
		{
			name: "since before all values",
			setup: func() *continuousAverager {
				a := NewAverager(dbRW, dbRO, "test_table", "since before all values").(*continuousAverager)
				if err := a.Observe(ctx, 1.0, WithCurrentTime(createTime(2))); err != nil {
					t.Fatalf("Observe(1.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 2.0, WithCurrentTime(createTime(3))); err != nil {
					t.Fatalf("Observe(2.0) returned error: %v", err)
				}
				return a
			},
			since:    createTime(1),
			expected: 1.5,
		},
		{
			name: "since after all values",
			setup: func() *continuousAverager {
				a := NewAverager(dbRW, dbRO, "test_table", "since after all values").(*continuousAverager)
				if err := a.Observe(ctx, 1.0, WithCurrentTime(createTime(1))); err != nil {
					t.Fatalf("Observe(1.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 2.0, WithCurrentTime(createTime(2))); err != nil {
					t.Fatalf("Observe(2.0) returned error: %v", err)
				}
				return a
			},
			since:    createTime(3),
			expected: 0.0,
		},
		{
			name: "wrapped buffer",
			setup: func() *continuousAverager {
				a := NewAverager(dbRW, dbRO, "test_table", "wrapped buffer").(*continuousAverager)
				if err := a.Observe(ctx, 1.0, WithCurrentTime(createTime(1))); err != nil {
					t.Fatalf("Observe(1.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 2.0, WithCurrentTime(createTime(2))); err != nil {
					t.Fatalf("Observe(2.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 3.0, WithCurrentTime(createTime(3))); err != nil {
					t.Fatalf("Observe(3.0) returned error: %v", err)
				}
				if err := a.Observe(ctx, 4.0, WithCurrentTime(createTime(4))); err != nil {
					t.Fatalf("Observe(4.0) returned error: %v", err)
				}
				return a
			},
			since:    createTime(2),
			expected: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup()
			result, err := a.Avg(ctx, WithSince(tt.since))
			if err != nil {
				t.Errorf("Read() returned error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Read() = %v, want %v", result, tt.expected)
			}
		})
	}
}
