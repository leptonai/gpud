package store

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

// BenchmarkInsertRealistic benchmarks realistic InfiniBand port insertion scenarios
// go test -v -run=^$ -bench=BenchmarkInsertRealistic -count=1 -timeout=10m -benchmem
func BenchmarkInsertRealistic(b *testing.B) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := openTestDBForBenchmark(b)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}

	// Create realistic test data representing a typical H100 cluster
	deviceNames := []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7"}

	// Pre-generate realistic port data
	testPorts := make([]infiniband.IBPort, len(deviceNames))
	for i, device := range deviceNames {
		// Most ports active with high performance
		state := "Active"
		physState := "LinkUp"
		rate := 400
		linkDown := uint64(0)

		// Some ports have issues (realistic distribution)
		if i == len(deviceNames)-1 || i == len(deviceNames)-2 {
			if i%2 == 0 {
				state = "Down"
				physState = "Disabled"
				rate = 200
				linkDown = uint64(25)
			} else {
				state = "Init"
				physState = "Polling"
				rate = 100
				linkDown = uint64(12)
			}
		}

		testPorts[i] = infiniband.IBPort{
			Device:          device,
			Port:            1,
			State:           state,
			PhysicalState:   physState,
			RateGBSec:       rate,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: linkDown,
		}
	}

	now := time.Now()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eventTime := now.Add(time.Duration(i) * time.Second)
		if err := store.Insert(eventTime, testPorts); err != nil {
			b.Fatalf("failed to insert: %v", err)
		}
	}
}
