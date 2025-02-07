package threshold

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/components/db"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestThresholdJSON(t *testing.T) {
	th := Threshold{
		AtLeastPorts: 4,
		AtLeastRate:  200,
	}

	// Test JSON marshaling
	b, err := th.JSON()
	require.NoError(t, err)

	// Test JSON unmarshaling
	parsed, err := Parse(b)
	require.NoError(t, err)
	assert.Equal(t, th.AtLeastPorts, parsed.AtLeastPorts)
	assert.Equal(t, th.AtLeastRate, parsed.AtLeastRate)

	// Test invalid JSON
	_, err = Parse([]byte("invalid json"))
	assert.Error(t, err)
}

func TestThresholdEvent(t *testing.T) {
	th := Threshold{
		AtLeastPorts: 4,
		AtLeastRate:  200,
	}

	now := time.Now().UTC()
	ev, err := th.Event(now)
	require.NoError(t, err)

	assert.Equal(t, metav1.NewTime(now), ev.Time)
	assert.Equal(t, "infiniband_threshold", ev.Name)
	assert.Equal(t, common.EventTypeInfo, ev.Type)
	assert.Contains(t, ev.Message, "4 ports")
	assert.Contains(t, ev.Message, "200 Gb/sec")

	// Verify the extra info contains valid JSON
	data := ev.ExtraInfo["data"]
	parsed, err := Parse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, th.AtLeastPorts, parsed.AtLeastPorts)
	assert.Equal(t, th.AtLeastRate, parsed.AtLeastRate)
}

func TestStore(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()
	eventsStore, err := db.NewStore(dbRW, dbRO, "test_infiniband_threshold", 0)
	require.NoError(t, err)
	store := NewStore(eventsStore)

	// Test initial state
	latest, err := store.Latest(ctx)
	require.NoError(t, err)
	assert.Nil(t, latest)

	// Test setting new threshold
	th1 := Threshold{AtLeastPorts: 4, AtLeastRate: 200}
	err = store.CompareAndSet(ctx, th1)
	require.NoError(t, err)

	latest, err = store.Latest(ctx)
	require.NoError(t, err)
	assert.Equal(t, th1.AtLeastPorts, latest.AtLeastPorts)
	assert.Equal(t, th1.AtLeastRate, latest.AtLeastRate)

	// Test setting same threshold (should not create new event)
	err = store.CompareAndSet(ctx, th1)
	require.NoError(t, err)

	events, err := eventsStore.Get(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(events)) // Should still have only one event

	// Test updating threshold
	th2 := Threshold{AtLeastPorts: 8, AtLeastRate: 400}
	err = store.CompareAndSet(ctx, th2)
	require.NoError(t, err)

	latest, err = store.Latest(ctx)
	require.NoError(t, err)
	assert.Equal(t, th2.AtLeastPorts, latest.AtLeastPorts)
	assert.Equal(t, th2.AtLeastRate, latest.AtLeastRate)
}

func TestDefaultStore(t *testing.T) {
	// Test initial state
	assert.Nil(t, GetDefaultStore())

	// Test setting default store
	store := &Store{}
	SetDefaultStore(store)
	assert.Equal(t, store, GetDefaultStore())

	// Test updating default store
	newStore := &Store{}
	SetDefaultStore(newStore)
	assert.Equal(t, newStore, GetDefaultStore())
}
