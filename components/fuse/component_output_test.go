package fuse

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	getFunc := CreateGet(Config{
		CongestedPercentAgainstThreshold:     90,
		MaxBackgroundPercentAgainstThreshold: 90,
	}, bucket)

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

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	// Test with low thresholds to trigger events
	cfg := Config{
		CongestedPercentAgainstThreshold:     10, // Low threshold to trigger event
		MaxBackgroundPercentAgainstThreshold: 10, // Low threshold to trigger event
	}

	getFunc := CreateGet(cfg, bucket)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := getFunc(ctx)
	require.NoError(t, err)

	output, ok := result.(*Output)
	require.True(t, ok)
	assert.NotNil(t, output)
	assert.NotNil(t, output.ConnectionInfos)

	// Check if events were created for exceeded thresholds
	events, err := bucket.Get(ctx, time.Now().Add(-1*time.Hour))
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

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	cfg := Config{
		CongestedPercentAgainstThreshold:     90,
		MaxBackgroundPercentAgainstThreshold: 90,
	}

	getFunc := CreateGet(cfg, bucket)

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
