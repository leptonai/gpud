package log

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/query"
	query_config "github.com/leptonai/gpud/pkg/query/config"
	query_log_common "github.com/leptonai/gpud/pkg/query/log/common"
	query_log_config "github.com/leptonai/gpud/pkg/query/log/config"
	query_log_tail "github.com/leptonai/gpud/pkg/query/log/tail"

	"github.com/nxadm/tail"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestPoller(t *testing.T) {
	t.Parallel()

	cfg := query_log_config.Config{
		File: "tail/testdata/kubelet.0.log",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	poller, err := newPoller(ctx, cfg, nil, nil)
	if err != nil {
		t.Fatalf("failed to create log poller: %v", err)
	}
	defer poller.Stop("test")

	if _, err := poller.Find(time.Now().Add(time.Hour)); err != query.ErrNoData {
		t.Fatalf("expected no data, got %v", err)
	}

	synced := 0

	poller.tailFileSeekInfoMu.Lock()
	poller.tailFileSeekInfoSyncer = func(_ context.Context, file string, seekInfo tail.SeekInfo) {
		synced++
		t.Logf("seek info: %+v", seekInfo)
	}
	poller.tailFileSeekInfoMu.Unlock()

	poller.Start(ctx, query_config.Config{Interval: metav1.Duration{Duration: time.Second}}, "test")

	time.Sleep(5 * time.Second)

	allItems, err := poller.Find(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("failed to get all items: %v", err)
	}
	for _, r := range allItems {
		t.Log(r.Line)
	}

	t.Logf("seek info %+v", poller.SeekInfo())

	if synced != 20 { // 20 lines
		t.Fatalf("expected 20 seek info sync, got %d", synced)
	}

	evs, err := poller.TailScan(ctx, query_log_tail.WithLinesToTail(1000))
	if err != nil {
		t.Fatalf("failed to tail: %v", err)
	}
	if len(evs) != 20 {
		t.Fatalf("expected 20 events, got %d", len(evs))
	}
}

func TestPollerTail(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(os.TempDir(), "test-log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	cfg := query_log_config.Config{
		File: f.Name(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	poller, err := newPoller(ctx, cfg, nil, nil)
	if err != nil {
		t.Fatalf("failed to create log poller: %v", err)
	}
	defer poller.Stop("test")

	synced := 0
	poller.tailFileSeekInfoMu.Lock()
	poller.tailFileSeekInfoSyncer = func(_ context.Context, file string, seekInfo tail.SeekInfo) {
		synced++
		t.Logf("seek info: %+v", seekInfo)
	}
	poller.tailFileSeekInfoMu.Unlock()

	poller.Start(ctx, query_config.Config{Interval: metav1.Duration{Duration: time.Second}}, "test")

	t.Log("writing 1")
	if _, err := f.WriteString("hello1\n"); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if ferr := f.Sync(); ferr != nil {
		t.Fatalf("failed to sync temp file: %v", ferr)
	}

	t.Log("writing 2")
	if _, err := f.WriteString("hello2\n"); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if ferr := f.Sync(); ferr != nil {
		t.Fatalf("failed to sync temp file: %v", ferr)
	}

	time.Sleep(10 * time.Second)

	allItems, err := poller.Find(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("failed to get all items: %v", err)
	}
	for _, r := range allItems {
		t.Log(r.Line)
	}

	t.Logf("seek info %+v", poller.SeekInfo())

	if synced != 2 { // 2 lines
		t.Fatalf("expected 2 seek info sync, got %d", synced)
	}

	evs, err := poller.TailScan(ctx, query_log_tail.WithLinesToTail(1000))
	if err != nil {
		t.Fatalf("failed to tail: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
}

func TestItemJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		item     Item
		wantErr  bool
		wantJSON string // Add expected JSON string for verification
		validate func(t *testing.T, got Item)
	}{
		{
			name: "basic item",
			item: Item{
				Time: metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				Line: "test log line",
			},
			wantJSON: `{"time":"2024-01-01T00:00:00Z","line":"test log line"}`,
			validate: func(t *testing.T, got Item) {
				if got.Line != "test log line" {
					t.Errorf("expected line %q, got %q", "test log line", got.Line)
				}
				if !got.Time.Equal(&metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}) {
					t.Errorf("expected time %v, got %v", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got.Time)
				}
			},
		},
		{
			name: "item with error",
			item: Item{
				Time:  metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				Line:  "test log line",
				Error: ptr.To("test error"),
			},
			wantJSON: `{"time":"2024-01-01T00:00:00Z","line":"test log line","error":"test error"}`,
			validate: func(t *testing.T, got Item) {
				if got.Error == nil || *got.Error != "test error" {
					t.Errorf("expected error %q, got %v", "test error", got.Error)
				}
			},
		},
		{
			name: "item with matched filter",
			item: Item{
				Time: metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				Line: "test log line",
				Matched: &query_log_common.Filter{
					Name:  "test filter",
					Regex: ptr.To("test.*"),
				},
			},
			wantJSON: `{"time":"2024-01-01T00:00:00Z","line":"test log line","matched":{"name":"test filter","regex":"test.*"}}`,
			validate: func(t *testing.T, got Item) {
				if got.Matched == nil {
					t.Fatal("expected matched filter, got nil")
				}
				if got.Matched.Name != "test filter" {
					t.Errorf("expected filter name %q, got %q", "test filter", got.Matched.Name)
				}
				if got.Matched.Regex == nil || *got.Matched.Regex != "test.*" {
					t.Errorf("expected filter regex %q, got %v", "test.*", got.Matched.Regex)
				}
			},
		},
		{
			name: "item with nil error",
			item: Item{
				Time:  metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				Line:  "test log line",
				Error: nil,
			},
			wantJSON: `{"time":"2024-01-01T00:00:00Z","line":"test log line"}`,
			validate: func(t *testing.T, got Item) {
				if got.Error != nil {
					t.Errorf("expected nil error, got %v", got.Error)
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test marshaling
			data, err := tc.item.JSON()
			if (err != nil) != tc.wantErr {
				t.Fatalf("JSON() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			// Verify JSON string matches expected
			if tc.wantJSON != "" {
				if got := string(data); got != tc.wantJSON {
					t.Errorf("JSON() = %v, want %v", got, tc.wantJSON)
				}
			}

			// Test unmarshaling
			got, err := ParseItemJSON(data)
			if err != nil {
				t.Fatalf("ParseItemJSON() error = %v", err)
			}

			// Run validation
			tc.validate(t, got)
		})
	}
}

func TestParseItemJSONErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "invalid json",
			input:   "invalid json",
			wantErr: true,
		},
		{
			name:    "empty json",
			input:   "{}",
			wantErr: false,
		},
		{
			name:    "invalid regex in filter",
			input:   `{"time":"2024-01-01T00:00:00Z","line":"test","matched":{"regex":"[invalid"}}`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseItemJSON([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseItemJSON() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
