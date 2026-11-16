package sqlite

import (
	"os"
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
		dbRW, _, cleanup := OpenTestDB(t)
		defer cleanup()
		var seq int
		var name, path string
		if err := dbRW.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
			t.Fatal(err)
		}
		if name != "main" || path == "" {
			t.Fatalf("unexpected database: %q %q", name, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("database file missing before cleanup: %v", err)
		}
		cleanup()
		// Other packages create databases concurrently. Check this database's path,
		// not the process-wide temp-directory file count.
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("database file still present after cleanup: %v", err)
		}
	})
}
