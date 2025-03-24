package gpudmetrics

import (
	"context"
	"testing"
	"time"

	metrics_state "github.com/leptonai/gpud/pkg/metrics/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	s := NewStore(dbRW, dbRO, "test_table", "test_name")
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestStoreObserve(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := metrics_state.CreateTableMetrics(ctx, dbRW, "test_table"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	a := NewStore(dbRW, dbRO, "test_table", "test_name")
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

	a := NewStore(dbRW, dbRO, "test_table", "test_name")
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
