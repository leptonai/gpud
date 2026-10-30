package eventstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestRecorderDedupPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		policy  DedupPolicy
		offsets []time.Duration
		want    int
	}{
		{"default minute", DedupPolicy{}, []time.Duration{0, 20 * time.Second, 70 * time.Second}, 2},
		{"generic five minutes", DedupPolicy{Window: 5 * time.Minute}, []time.Duration{0, 2 * time.Minute, 5 * time.Minute}, 2},
		{"window from first occurrence", DedupPolicy{EventWindow: func(Event) (time.Duration, bool) { return 5 * time.Minute, true }}, []time.Duration{0, 4 * time.Minute, 5 * time.Minute}, 2},
		{"unmatched override keeps generic", DedupPolicy{Window: 5 * time.Minute, EventWindow: func(Event) (time.Duration, bool) { return time.Minute, false }}, []time.Duration{0, 2 * time.Minute, 5 * time.Minute}, 2},
		{"invalid override keeps generic", DedupPolicy{EventWindow: func(Event) (time.Duration, bool) { return 0, true }}, []time.Duration{0, 20 * time.Second, 70 * time.Second}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()
			store, err := New(dbRW, dbRO, DefaultRetention)
			require.NoError(t, err)
			bucket, err := store.Bucket("recorder")
			require.NoError(t, err)
			defer bucket.Close()
			recorder := NewRecorder(bucket, tc.policy)
			base := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
			for _, offset := range tc.offsets {
				require.NoError(t, recorder.Record(t.Context(), Event{Time: base.Add(offset), Name: "event", Message: "normalized", Type: "warning"}))
			}
			events, err := bucket.Get(t.Context(), time.Unix(0, 0))
			require.NoError(t, err)
			assert.Len(t, events, tc.want)
			// Recreating GPUd's in-memory state must not duplicate an exact stored event.
			fresh := NewRecorder(bucket, tc.policy)
			require.NoError(t, fresh.Record(t.Context(), Event{Time: base, Name: "event", Message: "normalized", Type: "warning"}))
			events, err = bucket.Get(t.Context(), time.Unix(0, 0))
			require.NoError(t, err)
			assert.Len(t, events, tc.want)
		})
	}
}
func TestRecorderRawContentPolicy(t *testing.T) {
	base := time.Now()
	ordinary := NewRecorder(nil, DedupPolicy{})
	assert.True(t, ordinary.AcceptRaw(base, "same"))
	assert.False(t, ordinary.AcceptRaw(base, "same"))
	override := NewRecorder(nil, DedupPolicy{EventWindow: func(Event) (time.Duration, bool) { return time.Minute, true }})
	assert.True(t, override.AcceptRaw(base, "same"))
	assert.True(t, override.AcceptRaw(base, "same"))
}
