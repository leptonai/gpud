package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func createTestDB(t *testing.T) (*sql.DB, func()) {
	dbPath := "test.db"
	db, err := sqlite.Open(":memory:")
	assert.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(dbPath)
	}
}

func createTestEvent(timestamp time.Time) components.Event {
	return components.Event{
		Time:    metav1.Time{Time: timestamp},
		Name:    "test_event",
		Type:    "test_type",
		Message: "test message",
		ExtraInfo: map[string]string{
			"key": "value",
		},
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem},
		},
	}
}

func TestNew(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)
	assert.NotNil(t, store)
}

func TestCreateEvent(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	t.Run("create new event", func(t *testing.T) {
		event := createTestEvent(time.Now())
		id, err := store.CreateEvent(context.Background(), event)
		assert.NoError(t, err)
		assert.Greater(t, id, 0)
	})

	t.Run("create exact same event again", func(t *testing.T) {
		event := createTestEvent(time.Now())
		id1, err := store.CreateEvent(context.Background(), event)
		assert.NoError(t, err)
		assert.Greater(t, id1, 0)

		id2, err := store.CreateEvent(context.Background(), event)
		assert.NoError(t, err)
		assert.Equal(t, 0, id2)
	})
}

func TestGetEvents(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	now := time.Now()
	events := []components.Event{
		createTestEvent(now.Add(-2 * time.Hour)),
		createTestEvent(now.Add(-1 * time.Hour)),
		createTestEvent(now),
	}

	for _, event := range events {
		_, err := store.CreateEvent(context.Background(), event)
		assert.NoError(t, err)
	}

	t.Run("get all events", func(t *testing.T) {
		result, err := store.GetAllEvents(context.Background())
		assert.NoError(t, err)
		assert.Len(t, result, len(events))

		// 验证排序
		for i := 1; i < len(result); i++ {
			assert.True(t, result[i-1].Time.Before(&result[i].Time))
		}
	})

	t.Run("get events after", func(t *testing.T) {
		result, err := store.GetEvents(context.Background(), now.Add(-70*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, result, 2)
	})
}

func TestPurge(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	now := time.Now()
	oldEvent := createTestEvent(now.Add(-4 * 24 * time.Hour))
	newEvent := createTestEvent(now)

	_, err = store.CreateEvent(context.Background(), oldEvent)
	assert.NoError(t, err)
	_, err = store.CreateEvent(context.Background(), newEvent)
	assert.NoError(t, err)

	affected, err := store.purge(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, affected)

	events, err := store.GetAllEvents(context.Background())
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, newEvent.Time.Unix(), events[0].Time.Unix())
}

func TestConcurrency(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	concurrency := 10
	done := make(chan bool)

	for i := 0; i < concurrency; i++ {
		go func(i int) {
			event := createTestEvent(time.Now().Add(time.Duration(i) * time.Second))
			_, err := store.CreateEvent(context.Background(), event)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	events, err := store.GetAllEvents(context.Background())
	assert.NoError(t, err)
	assert.Len(t, events, concurrency)

	for i := 1; i < len(events); i++ {
		assert.True(t, events[i-1].Time.Before(&events[i].Time))
	}
}

func TestWarmupCache(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store1, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	event := createTestEvent(time.Now())
	_, err = store1.CreateEvent(context.Background(), event)
	assert.NoError(t, err)

	store2, err := New(context.Background(), db, "test_table")
	assert.NoError(t, err)

	events, err := store2.GetAllEvents(context.Background())
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, event.Name, events[0].Name)
}
