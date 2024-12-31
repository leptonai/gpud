package dmesg

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	var f *os.File
	var err error
	var component components.Component

	// Retry up to 2 times
	for attempt := 0; attempt < 3; attempt++ {
		func() {
			f, err = os.CreateTemp(os.TempDir(), "test-log")
			if err != nil {
				t.Logf("attempt %d: failed to create temp file: %v", attempt+1, err)
				return
			}
			defer os.Remove(f.Name())

			if _, err := f.WriteString("test\ntest\ntest"); err != nil {
				t.Logf("attempt %d: failed to write to temp file: %v", attempt+1, err)
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			xidErr := "NVRM: Xid (0000:03:00): 14, Channel 00000001"
			filters := []*query_log_common.Filter{
				{
					Name:      "xid error check",
					Substring: &xidErr,
				},
				{
					Name:  "oom 1",
					Regex: ptr.To(`^Out of memory:`),
				},
				{
					Name:  "oom 2",
					Regex: ptr.To(`\binvoked oom-killer\b`),
				},
			}

			pollInterval := 3 * time.Second
			component, err = New(
				ctx,
				Config{
					Log: query_log_config.Config{
						Query: query_config.Config{
							Interval: metav1.Duration{Duration: pollInterval},
						},
						File:          f.Name(),
						SelectFilters: filters,
					},
				},
				nil,
			)
			if err != nil {
				t.Logf("attempt %d: failed to create component: %v", attempt+1, err)
				return
			}

			t.Log("writing xid error")
			if _, err := f.WriteString(xidErr); err != nil {
				t.Logf("attempt %d: failed to write xid error: %v", attempt+1, err)
				return
			}
			time.Sleep(pollInterval + 5*time.Second)

			t.Log("writing OOM message 1")
			if _, err := f.WriteString("Out of memory: Killed process 123, UID 48, (httpd)."); err != nil {
				t.Logf("attempt %d: failed to write xid error: %v", attempt+1, err)
				return
			}
			time.Sleep(pollInterval + 5*time.Second)

			t.Log("writing OOM message 2")
			if _, err := f.WriteString("postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0"); err != nil {
				t.Logf("attempt %d: failed to write xid error: %v", attempt+1, err)
				return
			}
			time.Sleep(pollInterval + 5*time.Second)

			events, err := component.Events(ctx, time.Now().Add(-time.Hour))
			if err != nil {
				t.Logf("attempt %d: failed to get events: %v", attempt+1, err)
				return
			}
			t.Logf("events: %+v", events)

			if len(events) != 3 {
				t.Logf("attempt %d: expected 3 events, got %d", attempt+1, len(events))
				return
			}

			states, err := component.States(ctx)
			if err != nil {
				t.Logf("attempt %d: failed to get state: %v", attempt+1, err)
				return
			}
			t.Logf("states: %+v", states)

			parsedStates, err := ParseStates(states...)
			if err != nil {
				t.Logf("attempt %d: failed to parse states: %v", attempt+1, err)
				return
			}
			t.Logf("parsed states: %+v", parsedStates)
		}()

		// If no error occurred, break out of retry loop
		if err == nil {
			return
		}

		// Wait before retrying
		if attempt < 2 {
			time.Sleep(time.Second)
		}
	}

	// If we got here, all attempts failed
	t.Fatalf("test failed after 3 attempts, last error: %v", err)
}
