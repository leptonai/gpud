package tail

import (
	"context"
	"testing"
	"time"

	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"

	"k8s.io/utils/ptr"
)

// go test -bench=BenchmarkScan -benchmem
// go test -bench=BenchmarkScan_DmesgLog -benchmem
func BenchmarkScan_DmesgLog(b *testing.B) {
	ctx := context.Background()

	benchmarks := []struct {
		name        string
		linesToTail int
		withFilter  bool
		dedup       bool
	}{
		{"Tail100NoFilter", 100, false, false},
		{"Tail1000NoFilter", 1000, false, false},
		{"Tail100WithFilter", 100, true, false},
		{"Tail1000WithFilter", 1000, true, false},

		{"Tail100NoFilterWithDedup", 100, false, true},
		{"Tail1000NoFilterWithDedup", 1000, false, true},
		{"Tail100WithFilterWithDedup", 100, true, true},
		{"Tail1000WithFilterWithDedup", 1000, true, true},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var opts []OpOption
			opts = append(opts,
				WithFile("testdata/dmesg.0.log"),
				WithLinesToTail(bm.linesToTail),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, _ time.Time, _ *query_log_filter.Filter) {}),
			)

			if bm.withFilter {
				opts = append(opts, WithSelectFilter(&query_log_filter.Filter{
					Substring: ptr.To("error"),
				}))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Scan(ctx, opts...)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// go test -bench=BenchmarkScan -benchmem
// go test -bench=BenchmarkScan_KubeletLog -benchmem
func BenchmarkScan_KubeletLog(b *testing.B) {
	ctx := context.Background()

	benchmarks := []struct {
		name        string
		linesToTail int
		withFilter  bool
		dedup       bool
	}{
		{"Tail100NoFilter", 100, false, false},
		{"Tail1000NoFilter", 1000, false, false},
		{"Tail100WithFilter", 100, true, false},
		{"Tail1000WithFilter", 1000, true, false},

		{"Tail100NoFilterWithDedup", 100, false, true},
		{"Tail1000NoFilterWithDedup", 1000, false, true},
		{"Tail100WithFilterWithDedup", 100, true, true},
		{"Tail1000WithFilterWithDedup", 1000, true, true},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var opts []OpOption
			opts = append(opts,
				WithFile("testdata/kubelet.0.log"),
				WithLinesToTail(bm.linesToTail),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, _ time.Time, _ *query_log_filter.Filter) {}),
			)

			if bm.withFilter {
				opts = append(opts, WithSelectFilter(&query_log_filter.Filter{
					Substring: ptr.To("error"),
				}))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Scan(ctx, opts...)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
