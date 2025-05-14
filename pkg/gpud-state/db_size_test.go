package gpudstate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestRecordDBSize(t *testing.T) {
	t.Parallel()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create tables to ensure we have something to measure
	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Add some data to make the database non-empty
	err = SetMetadata(ctx, dbRW, MetadataKeyMachineID, "test-machine-id")
	require.NoError(t, err)
	err = SetMetadata(ctx, dbRW, MetadataKeyToken, "test-token")
	require.NoError(t, err)

	// Test recording db size
	err = RecordDBSize(ctx, dbRW)
	require.NoError(t, err)

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	err = RecordDBSize(canceledCtx, dbRW)
	assert.Error(t, err)
}
