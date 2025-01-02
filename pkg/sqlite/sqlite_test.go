package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sqlite_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "test.db")

	for _, dbFile := range []string{":memory:", dbFile} {
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

		// read-write mode
		dbRW, err := Open(dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer dbRW.Close()

		if _, err = dbRW.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
			t.Fatal(err)
		}
		if _, err = dbRW.Exec("INSERT INTO test (id, name) VALUES (1, 'test')"); err != nil {
			t.Fatal(err)
		}

		rows1, err := dbRO.Query("SELECT * FROM test")
		if err != nil {
			t.Fatal(err)
		}
		defer rows1.Close()

		rows2, err := dbRW.Query("SELECT * FROM test")
		if err != nil {
			t.Fatal(err)
		}
		defer rows2.Close()
	}
}
