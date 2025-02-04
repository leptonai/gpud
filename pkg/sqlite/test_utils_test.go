package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTestDB(t *testing.T) {
	t.Run("successful open and cleanup", func(t *testing.T) {
		dbRW, dbRO, cleanup := OpenTestDB(t)

		// Verify both connections are valid
		if err := dbRW.Ping(); err != nil {
			t.Errorf("RW database connection is not valid: %v", err)
		}
		if err := dbRO.Ping(); err != nil {
			t.Errorf("RO database connection is not valid: %v", err)
		}

		// Test write permission on RW connection
		_, err := dbRW.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Errorf("Failed to write to RW database: %v", err)
		}

		// Test read-only enforcement on RO connection
		_, err = dbRO.Exec("CREATE TABLE test2 (id INTEGER PRIMARY KEY)")
		if err == nil {
			t.Error("RO connection allowed write operation, expected error")
		}

		// Run cleanup
		cleanup()

		// Verify connections are closed
		if err := dbRW.Ping(); err == nil {
			t.Error("RW connection still open after cleanup")
		}
		if err := dbRO.Ping(); err == nil {
			t.Error("RO connection still open after cleanup")
		}
	})

	t.Run("cleanup removes temp file", func(t *testing.T) {
		// Get temp dir path for verification
		tempDir := os.TempDir()

		// Open test DB
		_, _, cleanup := OpenTestDB(t)

		// Count SQLite files in temp dir before cleanup
		beforeCount := countSQLiteFiles(t, tempDir)

		// Run cleanup
		cleanup()

		// Count SQLite files after cleanup
		afterCount := countSQLiteFiles(t, tempDir)

		if afterCount >= beforeCount {
			t.Error("Cleanup did not remove temporary database file")
		}
	})
}

// Helper function to count SQLite test files in directory
func countSQLiteFiles(t *testing.T, dir string) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read temp directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Base(entry.Name()) != "" && len(entry.Name()) >= 11 && entry.Name()[:11] == "test-sqlite" {
			count++
		}
	}
	return count
}
