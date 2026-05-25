package nfs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestCollectNFSHangEvents(t *testing.T) {
	now := time.Now()

	mk := func(name string, t time.Time) eventstore.Event {
		return eventstore.Event{
			Component: "nfs",
			Time:      t,
			Name:      name,
			Type:      "Warning",
			Message:   name,
		}
	}

	tests := []struct {
		name               string
		events             eventstore.Events
		wantHangCount      int
		wantReasonEmpty    bool
		wantReasonContains []string
	}{
		{
			name:            "empty input → no hang",
			events:          nil,
			wantHangCount:   0,
			wantReasonEmpty: true,
		},
		{
			name: "single lock reclaim → 1 hang",
			events: eventstore.Events{
				mk(eventNFSLockReclaimFailed, now.Add(-10*time.Minute)),
			},
			wantHangCount:      1,
			wantReasonContains: []string{"NFS hang detected", "1 lock reclaim failures"},
		},
		{
			name: "single not_responding with no ok → 1 hang",
			events: eventstore.Events{
				mk(eventNFSServerNotResponding, now.Add(-30*time.Minute)),
			},
			wantHangCount:      1,
			wantReasonContains: []string{"1 NFS server not-responding events"},
		},
		{
			name: "not_responding cancelled by later ok → 0 hang",
			events: eventstore.Events{
				// Descending order, as Bucket.Get would return.
				mk(eventNFSServerOK, now.Add(-30*time.Minute)),
				mk(eventNFSServerNotResponding, now.Add(-1*time.Hour)),
			},
			wantHangCount:   0,
			wantReasonEmpty: true,
		},
		{
			name: "not_responding later than ok → 1 hang",
			events: eventstore.Events{
				mk(eventNFSServerNotResponding, now.Add(-30*time.Minute)),
				mk(eventNFSServerOK, now.Add(-1*time.Hour)),
			},
			wantHangCount:      1,
			wantReasonContains: []string{"1 NFS server not-responding events"},
		},
		{
			name: "single writeback below threshold → 0 hang",
			events: eventstore.Events{
				mk(eventNFSWritebackHang, now.Add(-5*time.Minute)),
			},
			wantHangCount:   0,
			wantReasonEmpty: true,
		},
		{
			name: "two writeback events → 2 hang",
			events: eventstore.Events{
				mk(eventNFSWritebackHang, now.Add(-3*time.Minute)),
				mk(eventNFSWritebackHang, now.Add(-10*time.Minute)),
			},
			wantHangCount:      2,
			wantReasonContains: []string{"2 NFS writeback stack hints"},
		},
		{
			name: "1 lock_reclaim + 1 writeback → only lock_reclaim counts",
			events: eventstore.Events{
				mk(eventNFSLockReclaimFailed, now.Add(-5*time.Minute)),
				mk(eventNFSWritebackHang, now.Add(-10*time.Minute)),
			},
			wantHangCount:      1,
			wantReasonContains: []string{"1 lock reclaim failures"},
		},
		{
			name: "mixed all three rules → 6 hang, three reason segments",
			events: eventstore.Events{
				mk(eventNFSWritebackHang, now.Add(-1*time.Minute)),
				mk(eventNFSWritebackHang, now.Add(-2*time.Minute)),
				mk(eventNFSWritebackHang, now.Add(-3*time.Minute)),
				mk(eventNFSLockReclaimFailed, now.Add(-4*time.Minute)),
				mk(eventNFSServerNotResponding, now.Add(-5*time.Minute)),
				mk(eventNFSServerNotResponding, now.Add(-6*time.Minute)),
			},
			wantHangCount: 6,
			wantReasonContains: []string{
				"1 lock reclaim failures",
				"2 NFS server not-responding events",
				"3 NFS writeback stack hints",
				";",
			},
		},
		{
			name: "descending input yields ascending output",
			events: eventstore.Events{
				mk(eventNFSWritebackHang, now.Add(-1*time.Minute)),
				mk(eventNFSWritebackHang, now.Add(-5*time.Minute)),
				mk(eventNFSLockReclaimFailed, now.Add(-10*time.Minute)),
			},
			wantHangCount:      3,
			wantReasonContains: []string{"1 lock reclaim failures", "2 NFS writeback stack hints"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			hang, reason := collectNFSHangEvents(tc.events)

			assert.Equal(t, tc.wantHangCount, len(hang), "hang count mismatch")

			if tc.wantReasonEmpty {
				assert.Empty(t, reason, "expected empty reason")
				assert.Nil(t, hang, "expected nil hang slice when no hang detected")
			} else {
				for _, sub := range tc.wantReasonContains {
					assert.Contains(t, reason, sub, "reason missing substring %q", sub)
				}
			}

			// Ascending-order invariant (design doc §9 U2).
			for i := 0; i+1 < len(hang); i++ {
				cur := hang[i].Time
				next := hang[i+1].Time
				assert.True(t,
					cur.Before(next) || cur.Equal(next),
					"hang[%d].Time (%v) is after hang[%d].Time (%v); expected ascending order",
					i, cur, i+1, next,
				)
			}
		})
	}
}

func TestCollectNFSHangEventsServerOKIsPerServerAndPerCycle(t *testing.T) {
	now := time.Now()
	mk := func(name string, ts time.Time, message string) eventstore.Event {
		return eventstore.Event{
			Component: "nfs",
			Time:      ts,
			Name:      name,
			Type:      "Warning",
			Message:   message,
		}
	}

	hang, reason := collectNFSHangEvents(eventstore.Events{
		// A later OK from server-b must not cancel server-a.
		mk(eventNFSServerOK, now.Add(-10*time.Minute), messageNFSServerOK+": server-b"),
		// This is the currently unresolved server-a event.
		mk(eventNFSServerNotResponding, now.Add(-20*time.Minute), messageNFSServerNotResponding+": server-a"),
		// This older OK resolves only earlier server-a failures.
		mk(eventNFSServerOK, now.Add(-30*time.Minute), messageNFSServerOK+": server-a"),
		// This stale server-a event should not be carried into suggested actions.
		mk(eventNFSServerNotResponding, now.Add(-40*time.Minute), messageNFSServerNotResponding+": server-a"),
	})

	require.Len(t, hang, 1)
	assert.Equal(t, messageNFSServerNotResponding+": server-a", hang[0].Message)
	assert.Contains(t, reason, "1 NFS server not-responding events")
}
