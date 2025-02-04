package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "test.db")

	for _, dbFile := range []string{dbFile} {
		t.Run(dbFile, func(t *testing.T) {
			// Test read-only mode
			t.Run("read-only mode", func(t *testing.T) {
				dbRO, err := Open(dbFile, WithReadOnly(true))
				if err != nil {
					t.Fatal(err)
				}
				defer dbRO.Close()

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
				defer dbRW.Close()

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
				defer rows.Close()

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
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "size_test.db")
	db, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Test initial size
	t.Run("initial size", func(t *testing.T) {
		size, err := ReadDBSize(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if size == 0 {
			t.Error("expected non-zero initial size")
		}
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := ReadDBSize(ctx, db)
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
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "compact_test.db")
	db, err := Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
