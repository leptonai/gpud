package fabricmanager

import (
	"context"
	"os"
	"testing"
	"time"

	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	"github.com/leptonai/gpud/pkg/sqlite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentLog(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(os.TempDir(), "test-log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString("test\ntest\ntest\n"); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	pollInterval := 3 * time.Second
	component, err := New(
		ctx,
		Config{
			Log: query_log_config.Config{
				Query: query_config.Config{
					Interval: metav1.Duration{Duration: pollInterval},
					State: &query_config.State{
						DB: db,
					},
				},
				BufferSize:    query_log_config.DefaultBufferSize,
				File:          f.Name(),
				SelectFilters: filters,
			},
		},
	)

	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	t.Log("writing non error log")
	if _, err := f.WriteString("[Jul 24 2024 03:14:18] [INFO] [tid 855] Sending inband response message:  Message header details: magic Id:adbc request Id:2b59bdc21b9504c4 status:0 type:3 length:24\n"); err != nil {
		t.Fatalf("failed to write non error log: %v", err)
	}
	time.Sleep(pollInterval + 3*time.Second)

	t.Log("writing non fatal error log")
	if _, err := f.WriteString("[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61\n"); err != nil {
		t.Fatalf("failed to write non-fatal error log: %v", err)
	}
	time.Sleep(pollInterval + 3*time.Second)

	t.Log("writing fatal error log")
	if _, err := f.WriteString("[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33\n"); err != nil {
		t.Fatalf("failed to write fatal error log: %v", err)
	}
	time.Sleep(pollInterval + 3*time.Second)

	t.Log("writing fatal error log")
	if _, err := f.WriteString("[Apr 17 2024 01:51:39] [ERROR] [tid 2999877] failed to find the GPU handle 10187860174420860981 in the multicast team request setup 5653964288847403984.\n"); err != nil {
		t.Fatalf("failed to write fatal error log: %v", err)
	}
	time.Sleep(pollInterval + 3*time.Second)

	events, err := component.Events(ctx, time.Now().Add(-876600*time.Hour))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	t.Logf("events: %+v", events)

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}
