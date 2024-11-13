package log

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/leptonai/gpud/components/query"
	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"

	"github.com/nxadm/tail"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPoller(t *testing.T) {
	t.Parallel()

	cfg := query_log_config.Config{
		File: "tail/testdata/kubelet.0.log",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	poller, err := newPoller(ctx, cfg, nil)
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

	poller, err := newPoller(ctx, cfg, nil)
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
