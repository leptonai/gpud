package poller

import (
	"context"
	"testing"
	"time"

	poller_config "github.com/leptonai/gpud/poller/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BenchmarkProcessItem(b *testing.B) {
	now := time.Now()
	ctx := context.Background()

	benchmarks := []struct {
		name    string
		queueN  int
		preload int // number of items to preload before benchmark
	}{
		{"SmallQueue/Empty", 10, 0},
		{"SmallQueue/HalfFull", 10, 5},
		{"SmallQueue/Full", 10, 10},
		{"MediumQueue/Empty", 100, 0},
		{"MediumQueue/HalfFull", 100, 50},
		{"MediumQueue/Full", 100, 100},
		{"LargeQueue/Empty", 1000, 0},
		{"LargeQueue/HalfFull", 1000, 500},
		{"LargeQueue/Full", 1000, 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup poller with initial items
			q := &pollerImpl{
				ctx:       ctx,
				cfg:       poller_config.Config{QueueSize: bm.queueN},
				lastItems: make([]Item, 0, bm.queueN),
			}

			// Preload items
			for i := 0; i < bm.preload; i++ {
				q.processItem(Item{
					Time: metav1.NewTime(now.Add(time.Duration(-i) * time.Second)),
				})
			}

			// Reset timer before the actual benchmark
			b.ResetTimer()

			// Run benchmark
			for i := 0; i < b.N; i++ {
				q.processItem(Item{
					Time: metav1.NewTime(now.Add(time.Duration(i) * time.Second)),
				})
			}
		})
	}
}

func BenchmarkProcessItemParallel(b *testing.B) {
	now := time.Now()
	ctx := context.Background()

	benchmarks := []struct {
		name    string
		queueN  int
		preload int
	}{
		{"SmallQueue", 10, 5},
		{"MediumQueue", 100, 50},
		{"LargeQueue", 1000, 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			q := &pollerImpl{
				ctx:       ctx,
				cfg:       poller_config.Config{QueueSize: bm.queueN},
				lastItems: make([]Item, 0, bm.queueN),
			}

			// Preload items
			for i := 0; i < bm.preload; i++ {
				q.processItem(Item{
					Time: metav1.NewTime(now.Add(time.Duration(-i) * time.Second)),
				})
			}

			b.ResetTimer()

			// Run parallel benchmark
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					q.processItem(Item{
						Time: metav1.NewTime(now.Add(time.Duration(i) * time.Second)),
					})
					i++
				}
			})
		})
	}
}
