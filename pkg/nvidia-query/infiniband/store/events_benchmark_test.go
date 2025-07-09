package store

import (
	"context"
	"testing"
	"time"
)

// BenchmarkEventsQuery benchmarks querying events with realistic dataset sizes
// go test -v -run=^$ -bench=BenchmarkEventsQuery -count=1 -timeout=10m -benchmem
func BenchmarkEventsQuery(b *testing.B) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := openTestDBForBenchmark(b)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}

	// Pre-populate with realistic event data
	now := time.Now()
	for i := 0; i < 1000; i++ {
		eventTime := now.Add(time.Duration(i*30) * time.Second)
		if err := store.SetEventType("mlx5_0", 1, eventTime, "ib_port_drop", "Simulated drop event"); err != nil {
			b.Fatalf("failed to set event: %v", err)
		}
		if i%10 == 0 {
			if err := store.SetEventType("mlx5_1", 1, eventTime, "ib_port_flap", "Simulated flap event"); err != nil {
				b.Fatalf("failed to set flap event: %v", err)
			}
		}
	}

	queryTime := now.Add(-24 * time.Hour) // Query last 24 hours

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := store.Events(queryTime)
		if err != nil {
			b.Fatalf("failed to query events: %v", err)
		}
	}
}
