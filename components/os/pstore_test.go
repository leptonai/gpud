package os

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewComponent_PstoreInitialization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "gpud_pstore_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize the database
	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Initialize event store
	eventStore, err := eventstore.New(db, db, 24*time.Hour)
	require.NoError(t, err)

	// Create a test pstore directory with the testdata
	testPstoreDir := filepath.Join(tmpDir, "pstore")
	err = os.MkdirAll(testPstoreDir, 0755)
	require.NoError(t, err)

	// Copy testdata to the test pstore directory
	testdataDir := "../../pkg/pstore/testdata"
	err = copyTestdata(testdataDir, testPstoreDir)
	require.NoError(t, err)

	// Also copy files from subdirectory to root since pstore only scans root level files
	subDirPath := filepath.Join(testdataDir, "7530486857247")
	subDirEntries, err := os.ReadDir(subDirPath)
	if err == nil {
		for _, entry := range subDirEntries {
			if !entry.IsDir() {
				srcFile := filepath.Join(subDirPath, entry.Name())
				dstFile := filepath.Join(testPstoreDir, entry.Name())
				content, err := os.ReadFile(srcFile)
				if err == nil {
					require.NoError(t, os.WriteFile(dstFile, content, 0644))
				}
			}
		}
	}

	// Mock gpudInstance with event store
	gpudInstance := &components.GPUdInstance{
		RootCtx:    ctx,
		DBRW:       db,
		DBRO:       db,
		EventStore: eventStore,
	}

	// Create the component using newComponent with pstore directory
	comp, err := newComponent(gpudInstance, testPstoreDir)
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Cast to the internal component struct to access pstoreStore
	internalComp := comp.(*component)
	assert.NotNil(t, internalComp.pstoreStore)

	// Test that events were scanned and retrieved
	events, err := internalComp.pstoreStore.Get(ctx, time.Now().Add(-3*24*time.Hour))
	require.NoError(t, err)

	// Should have found events from testdata
	assert.Greater(t, len(events), 0, "Expected to find pstore events from testdata")

	// Verify event conversion and persistence to event store
	// Get events from the event store
	bucket, err := eventStore.Bucket("os")
	require.NoError(t, err)
	defer bucket.Close()

	since := time.Now().Add(-24 * time.Hour)
	storeEvents, err := bucket.Get(ctx, since)
	require.NoError(t, err)

	// Should have persisted events to the event store
	assert.Greater(t, len(storeEvents), 0, "Expected events to be persisted to event store")

	// Verify event structure
	for _, storeEvent := range storeEvents {
		// Note: Component field might be empty when retrieved from bucket since it's implicit
		assert.Equal(t, string(apiv1.EventTypeWarning), storeEvent.Type)
		assert.Equal(t, "kernel_panic", storeEvent.Name)
		assert.Contains(t, storeEvent.Message, "Kernel panic detected")
		assert.False(t, storeEvent.Time.IsZero())
	}
}

func TestNewComponent_PstoreDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "gpud_pstore_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize the database
	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Mock gpudInstance without EventStore to disable pstore (pstore is only initialized when EventStore != nil)
	gpudInstance := &components.GPUdInstance{
		RootCtx:    ctx,
		DBRW:       db,
		DBRO:       db,
		EventStore: nil, // This disables pstore initialization
	}

	// Create the component
	comp, err := newComponent(gpudInstance, "/some/pstore/dir")
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Cast to the internal component struct to check pstoreStore
	internalComp := comp.(*component)
	// pstore should be nil when EventStore is nil
	assert.Nil(t, internalComp.pstoreStore)
}

func TestNewComponent_PstoreError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "gpud_pstore_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize the database
	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Initialize event store
	eventStore, err := eventstore.New(db, db, 24*time.Hour)
	require.NoError(t, err)

	// Mock gpudInstance with event store
	gpudInstance := &components.GPUdInstance{
		RootCtx:    ctx,
		DBRW:       db,
		DBRO:       db,
		EventStore: eventStore,
	}

	// Create the component with invalid pstore directory
	comp, err := newComponent(gpudInstance, "/nonexistent/directory")

	// Should return error due to invalid pstore directory
	assert.Error(t, err)
	assert.Nil(t, comp)
}

// copyTestdata recursively copies files from source to destination
func copyTestdata(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyTestdata(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			srcFile, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, srcFile, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
