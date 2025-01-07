package peermem

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/components/dmesg"
	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *dmesg.State
		expected int
	}{
		{
			name: "should skip non-peermem invalid context events",
			input: &dmesg.State{
				TailScanMatched: []query_log.Item{
					{
						Time: metav1.Time{Time: time.Now()},
						Line: "some other event",
						Matched: &query_log_common.Filter{
							Name: "other_event",
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "should skip peermem invalid context events due to driver fix",
			input: &dmesg.State{
				TailScanMatched: []query_log.Item{
					{
						Time: metav1.Time{Time: time.Now()},
						Line: "nvidia-peermem invalid context",
						Matched: &query_log_common.Filter{
							Name: dmesg.EventNvidiaPeermemInvalidContext,
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "should deduplicate events in same minute",
			input: &dmesg.State{
				TailScanMatched: []query_log.Item{
					{
						Time: metav1.Time{Time: time.Now()},
						Line: "nvidia-peermem invalid context 1",
						Matched: &query_log_common.Filter{
							Name: dmesg.EventNvidiaPeermemInvalidContext,
						},
					},
					{
						Time: metav1.Time{Time: time.Now()},
						Line: "nvidia-peermem invalid context 2",
						Matched: &query_log_common.Filter{
							Name: dmesg.EventNvidiaPeermemInvalidContext,
						},
					},
				},
			},
			expected: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &component{}
			events, err := c.getEvents(context.Background(), time.Now(), tc.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(events) != tc.expected {
				t.Errorf("expected %d events, got %d", tc.expected, len(events))
			}
		})
	}
}
