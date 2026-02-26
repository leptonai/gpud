package eventstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestBucketWithDisablePurge(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 30*time.Second)
	require.NoError(t, err)

	bucket, err := store.Bucket("test_bucket_disable_purge", WithDisablePurge())
	require.NoError(t, err)
	defer bucket.Close()

	internal, ok := bucket.(*table)
	require.True(t, ok)
	assert.Equal(t, time.Duration(0), internal.retention)
	assert.Equal(t, time.Duration(0), internal.purgeInterval)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ev := Event{
		Time:    time.Now().UTC(),
		Name:    "test",
		Type:    string(apiv1.EventTypeInfo),
		Message: "disable purge bucket still writable",
	}
	require.NoError(t, bucket.Insert(ctx, ev))

	events, err := bucket.Get(ctx, time.Now().UTC().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ev.Name, events[0].Name)
	assert.Equal(t, ev.Type, events[0].Type)
	assert.Equal(t, ev.Message, events[0].Message)
}

func TestCreateTableWithClosedDB(t *testing.T) {
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	require.NoError(t, dbRW.Close())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := createTable(ctx, dbRW, "test_table_closed_db")
	require.Error(t, err)
}
