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

	id2, err := GetMachineID(ctx, dbRW)
	if err != nil {
		t.Fatal("failed to get machine id:", err)
	}
	if id2 != id {
		t.Fatalf("machine id mismatch: %s != %s", id2, id)
	}
}

func TestRecordMetrics(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, close := sqlite.OpenTestDB(t)
	defer close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := RecordMetrics(ctx, dbRO); err == nil {
		t.Fatal("expected error but got nil")
	}
	if err := RecordMetrics(ctx, dbRW); err != nil {
		t.Fatal("failed to record metrics:", err)
	}
}
