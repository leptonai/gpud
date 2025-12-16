package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dbFile := filepath.Join(tmpDir, "test.db")

	for _, dbFile := range []string{dbFile} {
		t.Run(dbFile, func(t *testing.T) {
			// Test read-only mode
			t.Run("read-only mode", func(t *testing.T) {
				dbRO, err := Open(dbFile, WithReadOnly(true))
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = dbRO.Close()
				}()

				if _, err = dbRO.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err == nil {
					t.Fatal("expected error when creating table in read-only mode, got nil")
				}
				if _, err = dbRO.Exec("INSERT INTO test (id, name) VALUES (1, 'test')"); err == nil {
					t.Fatal("expected error when inserting data in read-only mode, got nil")
				}
			})

			// Test read-write mode
			t.Run("read-write mode", func(t *testing.T) {
				dbRW, err := Open(dbFile)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = dbRW.Close()
				}()

				// Test table creation
				if _, err = dbRW.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
					t.Fatal(err)
				}

				// Test data insertion
				if _, err = dbRW.Exec("INSERT INTO test (id, name) VALUES (1, 'test')"); err != nil {
					t.Fatal(err)
				}

				// Test data reading
				rows, err := dbRW.Query("SELECT * FROM test")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = rows.Close()
				}()

				// Verify connection settings
				stats := dbRW.Stats()
				if stats.MaxOpenConnections != 1 {
					t.Errorf("expected MaxOpenConnections=1, got %d", stats.MaxOpenConnections)
				}
			})

			// Test non-existent file
			t.Run("non-existent file", func(t *testing.T) {
				nonExistentFile := filepath.Join(tmpDir, "non-existent.db")
				_, err := Open(nonExistentFile, WithReadOnly(true))
				if err != nil {
					t.Fatalf("failed to open non-existent file: %v", err)
				}
			})
		})
	}
}

func TestReadDBSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dbFile := filepath.Join(tmpDir, "size_test.db")

	// Create the database file first in read-write mode to ensure it exists
	dbInit, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	// Perform a simple operation to ensure the file is actually created
	_, err = dbInit.Exec("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	_ = dbInit.Close()

	ctx := context.Background()

	// Test initial size
	t.Run("initial size", func(t *testing.T) {
		dbRO, err := Open(dbFile, WithReadOnly(true))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = dbRO.Close()
		}()

		size, err := ReadDBSize(ctx, dbRO)
		if err != nil {
			t.Fatal(err)
		}
		if size == 0 {
			t.Error("expected non-zero initial size")
		}
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		dbRO, err := Open(dbFile, WithReadOnly(true))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = dbRO.Close()
		}()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = ReadDBSize(ctx, dbRO)
		if err == nil {
			t.Error("expected error with canceled context")
		}
	})
}

func TestCompact(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dbFile := filepath.Join(tmpDir, "compact_test.db")
	db, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx := context.Background()

	// Setup test data
	_, err = db.Exec("CREATE TABLE test_compact (id INTEGER PRIMARY KEY, data TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	// Insert and delete data to create free space
	t.Run("compact after delete", func(t *testing.T) {
		// Insert large data
		for i := 0; i < 100; i++ {
			_, err = db.Exec("INSERT INTO test_compact (data) VALUES (?)", strings.Repeat("x", 1000))
			if err != nil {
				t.Fatal(err)
			}
		}

		// Delete data to create free space
		_, err = db.Exec("DELETE FROM test_compact")
		if err != nil {
			t.Fatal(err)
		}

		sizeBeforeCompact, err := ReadDBSize(ctx, db)
		if err != nil {
			t.Fatal(err)
		}

		// Compact database
		err = Compact(ctx, db)
		if err != nil {
			t.Fatal(err)
		}

		sizeAfterCompact, err := ReadDBSize(ctx, db)
		if err != nil {
			t.Fatal(err)
		}

		if sizeAfterCompact >= sizeBeforeCompact {
			t.Error("expected size to decrease after compacting")
		}
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := Compact(ctx, db)
		if err == nil {
			t.Error("expected error with canceled context")
		}
	})
}

func TestRunCompact(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dbFile := filepath.Join(tmpDir, "runcompact_test.db")
	db, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Setup test data
	_, err = db.Exec("CREATE TABLE test_runcompact (id INTEGER PRIMARY KEY, data TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	// Insert large data
	for i := 0; i < 100; i++ {
		_, err = db.Exec("INSERT INTO test_runcompact (data) VALUES (?)", strings.Repeat("x", 1000))
		if err != nil {
			t.Fatal(err)
		}
	}

	// Delete data to create free space
	_, err = db.Exec("DELETE FROM test_runcompact")
	if err != nil {
		t.Fatal(err)
	}

	// Close the DB connection before running RunCompact
	_ = db.Close()

	t.Run("successful compaction", func(t *testing.T) {
		err := RunCompact(ctx, dbFile)
		if err != nil {
			t.Fatalf("RunCompact failed: %v", err)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non-existent.db")
		err := RunCompact(ctx, nonExistentFile)
		if err == nil {
			t.Error("expected error when compacting non-existent file")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := RunCompact(ctx, dbFile)
		if err == nil {
			t.Error("expected error with canceled context")
		}
	})

	t.Run("compacted size check", func(t *testing.T) {
		// Create a new DB with data and delete it again to ensure we have something to compact
		db, err := Open(dbFile)
		if err != nil {
			t.Fatal(err)
		}

		// Insert more data to ensure the DB needs compacting
		for i := 0; i < 100; i++ {
			_, err = db.Exec("INSERT INTO test_runcompact (data) VALUES (?)", strings.Repeat("y", 1000))
			if err != nil {
				t.Fatal(err)
			}
		}

		// Delete data
		_, err = db.Exec("DELETE FROM test_runcompact")
		if err != nil {
			t.Fatal(err)
		}

		_ = db.Close()

		// Get size before compaction
		dbRO, err := Open(dbFile, WithReadOnly(true))
		if err != nil {
			t.Fatal(err)
		}
		sizeBeforeCompact, err := ReadDBSize(ctx, dbRO)
		if err != nil {
			t.Fatal(err)
		}
		_ = dbRO.Close()

		// Run compaction
		err = RunCompact(ctx, dbFile)
		if err != nil {
			t.Fatalf("RunCompact failed: %v", err)
		}

		// Check size after compaction
		dbRO, err = Open(dbFile, WithReadOnly(true))
		if err != nil {
			t.Fatal(err)
		}
		sizeAfterCompact, err := ReadDBSize(ctx, dbRO)
		if err != nil {
			t.Fatal(err)
		}
		_ = dbRO.Close()

		if sizeAfterCompact >= sizeBeforeCompact {
			t.Errorf("expected size to decrease after compacting, before: %d, after: %d", sizeBeforeCompact, sizeAfterCompact)
		}
	})
}

func TestTableExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dbFile := filepath.Join(tmpDir, "table_exists_test.db")
	db, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx := context.Background()

	// Test non-existent table
	t.Run("non-existent table", func(t *testing.T) {
		exists, err := TableExists(ctx, db, "non_existent_table")
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Error("expected non-existent table to return false")
		}
	})

	// Test existing table
	t.Run("existing table", func(t *testing.T) {
		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, data TEXT)")
		if err != nil {
			t.Fatal(err)
		}
		exists, err := TableExists(ctx, db, "test_table")
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Error("expected existing table to return true")
		}
	})
}

func TestBuildConnectionString(t *testing.T) {
	t.Run("in-memory with shared cache produces file::memory:?cache=shared", func(t *testing.T) {
		// This is the exact connection string required for shared in-memory database
		// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
		conns, err := BuildConnectionString(":memory:", WithCache("shared"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(conns, "file::memory:?cache=shared"), "connection string should start with 'file::memory:?cache=shared', got: %s", conns)
		assert.Contains(t, conns, "_busy_timeout=5000")
		assert.Contains(t, conns, "_journal_mode=WAL")
		assert.Contains(t, conns, "_synchronous=NORMAL")
		assert.Contains(t, conns, "_txlock=immediate")
	})

	t.Run("in-memory with shared cache and read-only", func(t *testing.T) {
		conns, err := BuildConnectionString(":memory:", WithCache("shared"), WithReadOnly(true))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(conns, "file::memory:?cache=shared"), "connection string should start with 'file::memory:?cache=shared', got: %s", conns)
		assert.Contains(t, conns, "mode=ro")
		assert.NotContains(t, conns, "_txlock=immediate")
	})

	t.Run("in-memory without cache", func(t *testing.T) {
		conns, err := BuildConnectionString(":memory:")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(conns, "file::memory:?"), "connection string should start with 'file::memory:?', got: %s", conns)
		assert.NotContains(t, conns, "cache=")
	})

	t.Run("file-based database", func(t *testing.T) {
		conns, err := BuildConnectionString("/path/to/db.sqlite")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(conns, "file:/path/to/db.sqlite?"), "connection string should start with 'file:/path/to/db.sqlite?', got: %s", conns)
		assert.Contains(t, conns, "_busy_timeout=5000")
	})

	t.Run("file-based database ignores cache option", func(t *testing.T) {
		// Cache option is only meaningful for in-memory databases
		conns, err := BuildConnectionString("/path/to/db.sqlite", WithCache("shared"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(conns, "file:/path/to/db.sqlite?"), "connection string should start with 'file:/path/to/db.sqlite?', got: %s", conns)
		// Cache option should be ignored for file-based databases
		assert.NotContains(t, conns, "cache=shared")
	})
}

func TestOpenWithCache(t *testing.T) {
	t.Run("in-memory with shared cache allows multiple connections to share data", func(t *testing.T) {
		// Open RW connection with shared cache
		dbRW, err := Open(":memory:", WithCache("shared"))
		require.NoError(t, err)
		defer func() {
			_ = dbRW.Close()
		}()

		// Create a table in RW connection
		_, err = dbRW.Exec("CREATE TABLE test_shared (id INTEGER PRIMARY KEY, name TEXT)")
		require.NoError(t, err)

		// Insert data
		_, err = dbRW.Exec("INSERT INTO test_shared (id, name) VALUES (1, 'test')")
		require.NoError(t, err)

		// Open RO connection with same shared cache
		dbRO, err := Open(":memory:", WithCache("shared"), WithReadOnly(true))
		require.NoError(t, err)
		defer func() {
			_ = dbRO.Close()
		}()

		// Verify RO connection can see the data (shared cache works)
		var name string
		err = dbRO.QueryRow("SELECT name FROM test_shared WHERE id = 1").Scan(&name)
		require.NoError(t, err, "RO connection should see data from shared cache")
		assert.Equal(t, "test", name)
	})

	t.Run("in-memory without shared cache creates separate databases", func(t *testing.T) {
		dbRW, err := Open(":memory:")
		require.NoError(t, err)
		defer func() {
			_ = dbRW.Close()
		}()

		_, err = dbRW.Exec("CREATE TABLE test_separate (id INTEGER PRIMARY KEY)")
		require.NoError(t, err)

		// Another connection without shared cache should have separate database
		dbRW2, err := Open(":memory:")
		require.NoError(t, err)
		defer func() {
			_ = dbRW2.Close()
		}()

		// Table should not exist in second connection
		exists, err := TableExists(context.Background(), dbRW2, "test_separate")
		require.NoError(t, err)
		assert.False(t, exists, "without shared cache, separate connections should have separate databases")
	})

	t.Run("file-based database works with cache option", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_cache_test")
		require.NoError(t, err)
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		dbFile := filepath.Join(tmpDir, "test.db")
		db, err := Open(dbFile, WithCache("shared"))
		require.NoError(t, err)
		defer func() {
			_ = db.Close()
		}()

		// Should work normally, cache option is ignored for file-based
		_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
		require.NoError(t, err)
	})
}
