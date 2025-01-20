package state

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/prometheus/client_golang/prometheus"
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

	reg := prometheus.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatal("failed to register metrics:", err)
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := CreateTableMachineMetadata(ctx, dbRW); err != nil {
		t.Fatal("failed to create table:", err)
	}
	id, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	if err != nil {
		t.Fatal("failed to create machine id:", err)
	}
	t.Log(id)

	if err := RecordMetrics(ctx, dbRO); err != nil {
		t.Fatal("failed to record metrics:", err)
	}
	if err := Compact(ctx, dbRW); err != nil {
		t.Fatal("failed to compact database:", err)
	}

	size, err := sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatal("failed to read db size:", err)
	}
	t.Log(size)
}
