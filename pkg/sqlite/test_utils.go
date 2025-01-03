package sqlite

import (
	"database/sql"
	"os"
	"testing"
)

func OpenTestDB(t *testing.T) (*sql.DB, *sql.DB, func()) {
	tmpf, err := os.CreateTemp(os.TempDir(), "test-sqlite")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	dbRW, err := Open(tmpf.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	dbRO, err := Open(tmpf.Name(), WithReadOnly(true))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	return dbRW, dbRO, func() {
		_ = dbRW.Close()
		_ = dbRO.Close()
		_ = os.Remove(tmpf.Name())
	}
}
