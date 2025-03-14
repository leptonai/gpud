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

	t.Logf("AvgSince result: %f", avgResult)

	expectedAvg := 250.5 // (1 + 500) / 2
	if avgResult != expectedAvg {
		t.Errorf("AvgSince() = %f; want %f", avgResult, expectedAvg)
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

func TestNoOpAverager(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := NewNoOpAverager()

	// Test MetricName
	if name := a.MetricName(); name != "" {
		t.Errorf("MetricName() = %q, want empty string", name)
	}

	// Test Last
	value, ok, err := a.Last(ctx)
	if err != nil {
		t.Errorf("Last() returned error: %v", err)
	}
	if ok {
		t.Errorf("Last() returned ok = true, want false")
	}
	if value != 0 {
		t.Errorf("Last() returned value = %f, want 0", value)
	}

	// Test Last with options - should still return zero values
	value, ok, err = a.Last(ctx, WithSince(time.Now()), WithMetricSecondaryName("test"))
	if err != nil {
		t.Errorf("Last() with options returned error: %v", err)
	}
	if ok {
		t.Errorf("Last() with options returned ok = true, want false")
	}
	if value != 0 {
		t.Errorf("Last() with options returned value = %f, want 0", value)
	}

	// Test Observe - should do nothing and return nil
	if err := a.Observe(ctx, 123.45); err != nil {
		t.Errorf("Observe() returned error: %v", err)
	}

	// Test Observe with options - should still do nothing and return nil
	if err := a.Observe(ctx, 456.78, WithCurrentTime(time.Now()), WithMetricSecondaryName("test")); err != nil {
		t.Errorf("Observe() with options returned error: %v", err)
	}

	// Test Avg - should return zero and nil error
	avg, err := a.Avg(ctx)
	if err != nil {
		t.Errorf("Avg() returned error: %v", err)
	}
	if avg != 0 {
		t.Errorf("Avg() returned %f, want 0", avg)
	}

	// Test Avg with options - should still return zero and nil error
	avg, err = a.Avg(ctx, WithSince(time.Now()))
	if err != nil {
		t.Errorf("Avg() with options returned error: %v", err)
	}
	if avg != 0 {
		t.Errorf("Avg() with options returned %f, want 0", avg)
	}

	// Test Read - should return empty metrics and nil error
	metrics, err := a.Read(ctx)
	if err != nil {
		t.Errorf("Read() returned error: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("Read() returned %d metrics, want 0", len(metrics))
	}

	// Test Read with options - should still return empty metrics and nil error
	metrics, err = a.Read(ctx, WithSince(time.Now()), WithMetricSecondaryName("test"))
	if err != nil {
		t.Errorf("Read() with options returned error: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("Read() with options returned %d metrics, want 0", len(metrics))
	}
}
