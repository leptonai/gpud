package state

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/google/uuid"
)

func TestBootIDs(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := CreateTableBootIDs(ctx, db); err != nil {
		t.Fatal("failed to create table:", err)
	}

	id, err := GetLastBootID(ctx, db)
	if err != nil {
		t.Fatal("failed to get last boot id:", err)
	}
	if id != "" {
		t.Fatal("expected empty boot id, got:", id)
	}

	first := time.Now().UTC()

	uuid1 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid1, first); err != nil {
		t.Fatal("failed to insert boot id:", err)
	}

	id, err = GetLastBootID(ctx, db)
	if err != nil {
		t.Fatal("failed to get last boot id:", err)
	}
	if id != uuid1 {
		t.Fatal("expected boot id:", uuid1, "got:", id)
	}

	// insert the same boot id again, and should fail due to unique constraint
	err = InsertBootID(ctx, db, uuid1, first.Add(1*time.Second))
	if err == nil {
		t.Fatal("expected unique constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatal("expected unique constraint violation, got:", err)
	}

	uuid2 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid2, first.Add(1*time.Second)); err != nil {
		t.Fatal("failed to insert boot id:", err)
	}

	uuid3 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid3, first.Add(2*time.Second)); err != nil {
		t.Fatal("failed to insert boot id:", err)
	}

	events, err := GetRebootEvents(ctx, db, time.Now().Add(-30*time.Second))
	if err != nil {
		t.Fatal("failed to get reboot events:", err)
	}
	if len(events) != 2 {
		t.Fatal("expected 1 reboot event, got:", len(events))
	}
	if events[0].BootID != uuid2 {
		t.Fatal("expected boot id:", uuid2, "got:", events[0].BootID)
	}
	if events[1].BootID != uuid3 {
		t.Fatal("expected boot id:", uuid3, "got:", events[1].BootID)
	}
}
