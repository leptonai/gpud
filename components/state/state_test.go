package state

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestOpenMemory(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// create some table
	if _, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal("failed to create table:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := CreateTableMachineMetadata(ctx, db); err != nil {
		t.Fatal("failed to create table:", err)
	}
	id, err := CreateMachineIDIfNotExist(ctx, db, "")
	if err != nil {
		t.Fatal("failed to create machine id:", err)
	}
	t.Log(id)
}
