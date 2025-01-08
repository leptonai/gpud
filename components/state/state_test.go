package state

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestOpenMemory(t *testing.T) {
	t.Parallel()

	dbRW, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbRW.Close()

	// create some table
	if _, err = dbRW.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal("failed to create table:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := CreateTableMachineMetadata(ctx, dbRW); err != nil {
		t.Fatal("failed to create table:", err)
	}
	id, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	if err != nil {
		t.Fatal("failed to create machine id:", err)
	}
	t.Log(id)
}
