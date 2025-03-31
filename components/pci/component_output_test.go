package pci

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/eventstore"
	query_config "github.com/leptonai/gpud/pkg/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	getFunc := CreateGet(bucket)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err = getFunc(ctx); err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
}

func TestDefaultPoller(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	// Test initial state
	assert.Nil(t, getDefaultPoller())

	// Test setting default poller
	cfg := Config{
		Query: query_config.Config{},
	}
	setDefaultPoller(cfg, bucket)
	assert.NotNil(t, getDefaultPoller())

	// Test that calling setDefaultPoller again doesn't change the poller (sync.Once)
	originalPoller := getDefaultPoller()
	setDefaultPoller(cfg, bucket)
	assert.Equal(t, originalPoller, getDefaultPoller())
}

func TestCreateGetWithContextTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	getFunc := CreateGet(bucket)

	// create an already canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = getFunc(ctx)
	assert.Error(t, err)
}
