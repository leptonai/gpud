package fuse

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/common"
	events_db "github.com/leptonai/gpud/internal/db"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	if err != nil {
		t.Fatalf("failed to create events store: %v", err)
	}
	defer eventsStore.Close()

	getFunc := CreateGet(Config{
		CongestedPercentAgainstThreshold:     90,
		MaxBackgroundPercentAgainstThreshold: 90,
	}, eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
}

func TestCreateGetWithThresholds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	// Test with low thresholds to trigger events
	cfg := Config{
		CongestedPercentAgainstThreshold:     10, // Low threshold to trigger event
		MaxBackgroundPercentAgainstThreshold: 10, // Low threshold to trigger event
	}

	getFunc := CreateGet(cfg, eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := getFunc(ctx)
	require.NoError(t, err)

	output, ok := result.(*Output)
	require.True(t, ok)
	assert.NotNil(t, output)
	assert.NotNil(t, output.ConnectionInfos)

	// Check if events were created for exceeded thresholds
	events, err := eventsStore.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	for _, event := range events {
		assert.Equal(t, "fuse_connections", event.Name)
		assert.Equal(t, common.EventTypeCritical, event.Type)
		assert.Contains(t, event.Message, "percent against threshold")
		assert.NotEmpty(t, event.ExtraInfo["data"])
		assert.Equal(t, "json", event.ExtraInfo["encoding"])
	}
}

func TestCreateGetDeduplication(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	require.NoError(t, err)
	defer eventsStore.Close()

	cfg := Config{
		CongestedPercentAgainstThreshold:     90,
		MaxBackgroundPercentAgainstThreshold: 90,
	}

	getFunc := CreateGet(cfg, eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := getFunc(ctx)
	require.NoError(t, err)

	output, ok := result.(*Output)
	require.True(t, ok)

	// Check for duplicate device names
	seenDevices := make(map[string]bool)
	for _, info := range output.ConnectionInfos {
		if seenDevices[info.DeviceName] {
			t.Errorf("Duplicate device name found: %s", info.DeviceName)
		}
		seenDevices[info.DeviceName] = true
	}
}
