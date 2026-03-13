package store

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// BenchmarkScanRealistic benchmarks the event scanning with realistic data patterns
// go test -v -run=^$ -bench=BenchmarkScanRealistic -count=1 -timeout=10m -benchmem
func BenchmarkScanRealistic(b *testing.B) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := openTestDBForBenchmark(b)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}

	// Create a dataset with realistic drop/flap patterns
	deviceNames := []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3"}
	now := time.Now()
	linkDownCounts := make([]uint64, 100)
	for i := range linkDownCounts {
		if i > 0 {
			linkDownCounts[i] = linkDownCounts[i-1]
		}
		if i > 0 && i%10 == 0 {
			linkDownCounts[i]++
		}
	}

	// Insert baseline data with some problematic patterns
	for i := range 100 {
		eventTime := now.Add(time.Duration(i*30) * time.Second) // Every 30s

		ports := make([]types.IBPort, len(deviceNames))
		for j, device := range deviceNames {
			state := "Active"
			physState := "LinkUp"
			linkDown := linkDownCounts[i] // Gradual increase

			// Simulate flapping on device 1
			if j == 1 && i%10 < 3 {
				state = "Down"
				physState = "Disabled"
			}

			// Simulate drops on device 2 with higher frequency
			if j == 2 && i%5 == 0 {
				linkDown += 5
			}

			ports[j] = types.IBPort{
				Device:          device,
				Port:            1,
				State:           state,
				PhysicalState:   physState,
				RateGBSec:       400,
				LinkLayer:       "Infiniband",
				TotalLinkDowned: linkDown,
			}
		}

		if err := store.Insert(eventTime, ports); err != nil {
			b.Fatalf("failed to insert test data: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if err := store.Scan(); err != nil {
			b.Fatalf("failed to scan: %v", err)
		}
	}
}
