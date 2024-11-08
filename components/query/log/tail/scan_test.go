package tail

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	"k8s.io/utils/ptr"
)

func TestScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	content := "line1\nline2\nline3\nline4\nline5\n"
	if _, err := tmpf.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpf.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	t.Logf("wrote %q", tmpf.Name())

	largeTmpf, err := os.CreateTemp("", "large_test*.txt")
	if err != nil {
		t.Fatalf("failed to create large temp file: %v", err)
	}
	defer os.Remove(largeTmpf.Name())

	// Write 1000 lines to the large file
	for i := 1; i <= 1000; i++ {
		if _, err := largeTmpf.WriteString(fmt.Sprintf("line%d\n", i)); err != nil {
			t.Fatalf("failed to write to large temp file: %v", err)
		}
	}
	if err := largeTmpf.Close(); err != nil {
		t.Fatalf("failed to close large temp file: %v", err)
	}

	tests := []struct {
		name          string
		fileName      string
		commandArgs   []string
		n             int
		selectFilters []*query_log_common.Filter
		want          []string
		wantError     bool
	}{
		{
			name:     "tail 3 lines",
			fileName: tmpf.Name(),
			n:        3,
			want:     []string{"line5", "line4", "line3"},
		},
		{
			name:     "tail more lines than file contains",
			fileName: tmpf.Name(),
			n:        10,
			want:     []string{"line5", "line4", "line3", "line2", "line1"},
		},
		{
			name:     "tail with match function",
			fileName: tmpf.Name(),
			n:        3,
			selectFilters: []*query_log_common.Filter{
				{Regex: ptr.To("3")},
				{Regex: ptr.To("5")},
			},
			want: []string{"line5", "line3"},
		},
		{
			name:     "tail with match function",
			fileName: tmpf.Name(),
			n:        3,
			selectFilters: []*query_log_common.Filter{
				{Substring: ptr.To("3")},
				{Substring: ptr.To("5")},
			},
			want: []string{"line5", "line3"},
		},
		{
			name:      "non-existent file",
			fileName:  "non-existent_file",
			n:         3,
			wantError: true,
		},

		{
			name:     "tail 100 lines from large file",
			fileName: largeTmpf.Name(),
			n:        100,
			want:     generateExpectedLines(1000, 100),
		},
		{
			name:        "tail 100 lines from large file but with cat",
			commandArgs: []string{"cat", largeTmpf.Name()},
			n:           100,
			want:        generateExpectedLines(1000, 100),
		},

		{
			name:     "tail 1000 lines from large file",
			fileName: largeTmpf.Name(),
			n:        1000,
			want:     generateExpectedLines(1000, 1000),
		},
		{
			name:        "tail 1000 lines from large file but with cat",
			commandArgs: []string{"cat", largeTmpf.Name()},
			n:           1000,
			want:        generateExpectedLines(1000, 1000),
		},

		{
			name:     "tail with regex filter on large file",
			fileName: largeTmpf.Name(),
			n:        1000,
			selectFilters: []*query_log_common.Filter{
				{Regex: ptr.To("line(50|100|150)")},
			},
			want: []string{"line1000", "line509", "line508", "line507", "line506", "line505", "line504", "line503", "line502", "line501", "line500", "line150", "line100", "line50"},
		},
		{
			name:        "tail with regex filter on large file but with cat",
			commandArgs: []string{"cat", largeTmpf.Name()},
			n:           1000,
			selectFilters: []*query_log_common.Filter{
				{Regex: ptr.To("line(50|100|150)")},
			},
			want: []string{"line1000", "line509", "line508", "line507", "line506", "line505", "line504", "line503", "line502", "line501", "line500", "line150", "line100", "line50"},
		},

		{
			name:     "tail kubelet.0.log",
			fileName: "testdata/kubelet.0.log",
			n:        5,
			want:     nil, // We'll check the length instead of exact content
		},
		{
			name:        "tail kubelet.0.log but with cat",
			commandArgs: []string{"cat", "testdata/kubelet.0.log"},
			n:           5,
			want:        nil, // We'll check the length instead of exact content
		},

		{
			name:     "tail kubelet.0.log with filter",
			fileName: "testdata/kubelet.0.log",
			n:        1000,
			selectFilters: []*query_log_common.Filter{
				{Substring: ptr.To("error")},
			},
			want: nil, // We'll check the length instead of exact content
		},
		{
			name:        "tail kubelet.0.log with filter but with cat",
			commandArgs: []string{"cat", "testdata/kubelet.0.log"},
			n:           1000,
			selectFilters: []*query_log_common.Filter{
				{Substring: ptr.To("error")},
			},
			want: nil, // We'll check the length instead of exact content
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			_, err := Scan(
				ctx,
				WithFile(tt.fileName),
				WithCommands([][]string{tt.commandArgs}),
				WithLinesToTail(tt.n),
				WithSelectFilter(tt.selectFilters...),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, time time.Time, filter *query_log_common.Filter) {
					got = append(got, string(line))
				}),
			)

			if (err != nil) != tt.wantError {
				t.Errorf("Scan = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.fileName == "testdata/kubelet.0.log" || strings.Contains(strings.Join(tt.commandArgs, " "), "testdata/kubelet.0.log") {
				// For kubelet.0.log, we'll just check if we got any results
				if len(got) == 0 {
					t.Errorf("Scan on kubelet.0.log returned no results")
				}
				if tt.selectFilters != nil && len(got) == 0 {
					t.Errorf("Scan on kubelet.0.log with filter returned no results")
				}
			} else if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Scan = %v, want %v", got, tt.want)
			}
		})
	}
}

func generateExpectedLines(total, n int) []string {
	var result []string
	for i := total; i > total-n && i > 0; i-- {
		result = append(result, fmt.Sprintf("line%d", i))
	}
	return result
}

func TestScan_LastLineWithoutNewline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create temp file with content that doesn't end in newline
	tmpf, err := os.CreateTemp("", "test_nonewline*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	// Write content without final newline
	content := "line1\nline2\nline3\nfinal_line_no_newline"
	if _, err := tmpf.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpf.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	tests := []struct {
		name          string
		linesToTail   int
		selectFilters []*query_log_filter.Filter
		want          []string
	}{
		{
			name:        "tail 2 lines with last line having no newline",
			linesToTail: 2,
			want:        []string{"final_line_no_newline", "line3"},
		},
		{
			name:        "tail all lines with last line having no newline",
			linesToTail: 5,
			want:        []string{"final_line_no_newline", "line3", "line2", "line1"},
		},
		{
			name:        "tail with filter matching last line",
			linesToTail: 5,
			selectFilters: []*query_log_filter.Filter{
				{Substring: ptr.To("final")},
			},
			want: []string{"final_line_no_newline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			_, err := Scan(
				ctx,
				WithFile(tmpf.Name()),
				WithLinesToTail(tt.linesToTail),
				WithSelectFilter(tt.selectFilters...),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, time time.Time, filter *query_log_filter.Filter) {
					got = append(got, string(line))
				}),
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Scan = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScan_Dedup(t *testing.T) {
	ctx := context.Background()

	// Create temp file with duplicate lines
	tmpf, err := os.CreateTemp("", "test_dedup*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	// Write content with duplicate lines in different patterns
	content := strings.Join([]string{
		"unique_line_1",
		"duplicate_line",
		"unique_line_2",
		"duplicate_line", // Immediate duplicate
		"unique_line_3",
		"duplicate_line", // Distant duplicate
		"unique_line_4",
		"DUPLICATE_LINE", // Case different but same content when lowercased
		"unique_line_5",
		"duplicate_line\n", // With trailing newline
		"unique_line_6",
	}, "\n")

	if _, err := tmpf.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpf.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	tests := []struct {
		name        string
		linesToTail int
		dedup       bool
		want        []string
		wantCount   int
	}{
		{
			name:        "no dedup",
			linesToTail: 100,
			dedup:       false,
			want: []string{
				"unique_line_6",
				"duplicate_line",
				"unique_line_5",
				"DUPLICATE_LINE",
				"unique_line_4",
				"duplicate_line",
				"unique_line_3",
				"duplicate_line",
				"unique_line_2",
				"duplicate_line",
				"unique_line_1",
			},
			wantCount: 11,
		},
		{
			name:        "with dedup",
			linesToTail: 100,
			dedup:       true,
			want: []string{
				"unique_line_6",
				"duplicate_line",
				"unique_line_5",
				"DUPLICATE_LINE",
				"unique_line_4",
				"unique_line_3",
				"unique_line_2",
				"unique_line_1",
			},
			wantCount: 8,
		},
		{
			name:        "dedup with limited lines",
			linesToTail: 5,
			dedup:       true,
			want: []string{
				"unique_line_6",
				"duplicate_line",
				"unique_line_5",
				"DUPLICATE_LINE",
				"unique_line_4",
			},
			wantCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			count, err := Scan(
				ctx,
				WithFile(tmpf.Name()),
				WithLinesToTail(tt.linesToTail),
				WithDedup(tt.dedup),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, time time.Time, filter *query_log_filter.Filter) {
					got = append(got, string(line))
				}),
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if count != tt.wantCount {
				t.Errorf("got count = %d, want %d", count, tt.wantCount)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got lines = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScan_DedupWithFilters(t *testing.T) {
	ctx := context.Background()

	// Create temp file with duplicate lines and different patterns
	tmpf, err := os.CreateTemp("", "test_dedup_filter*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	content := strings.Join([]string{
		"error: duplicate error message",
		"info: some info",
		"error: duplicate error message",
		"warning: some warning",
		"error: different error",
		"error: duplicate error message",
		"info: another info",
	}, "\n")

	if _, err := tmpf.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpf.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	tests := []struct {
		name          string
		linesToTail   int
		dedup         bool
		selectFilters []*query_log_filter.Filter
		want          []string
		wantCount     int
	}{
		{
			name:        "filter without dedup",
			linesToTail: 100,
			dedup:       false,
			selectFilters: []*query_log_filter.Filter{
				{Substring: ptr.To("error")},
			},
			want: []string{
				"error: duplicate error message",
				"error: different error",
				"error: duplicate error message",
				"error: duplicate error message",
			},
			wantCount: 4,
		},
		{
			name:        "filter with dedup",
			linesToTail: 100,
			dedup:       true,
			selectFilters: []*query_log_filter.Filter{
				{Substring: ptr.To("error")},
			},
			want: []string{
				"error: duplicate error message",
				"error: different error",
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			count, err := Scan(
				ctx,
				WithFile(tmpf.Name()),
				WithLinesToTail(tt.linesToTail),
				WithDedup(tt.dedup),
				WithSelectFilter(tt.selectFilters...),
				WithParseTime(func(line []byte) (time.Time, error) {
					return time.Time{}, nil
				}),
				WithProcessMatched(func(line []byte, time time.Time, filter *query_log_filter.Filter) {
					got = append(got, string(line))
				}),
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if count != tt.wantCount {
				t.Errorf("got count = %d, want %d", count, tt.wantCount)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got lines = %v, want %v", got, tt.want)
			}
		})
	}
}

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
