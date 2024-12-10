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
		t.Fatalf("failed to create table: %v", err)
	}

	id, err := GetLastBootID(ctx, db)
	if err != nil {
		t.Fatalf("failed to get last boot id: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty boot id, got: %q", id)
	}

	first := time.Now().UTC()

	uuid1 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid1, first); err != nil {
		t.Fatalf("failed to insert boot id: %v", err)
	}

	id, err = GetLastBootID(ctx, db)
	if err != nil {
		t.Fatalf("failed to get last boot id: %v", err)
	}
	if id != uuid1 {
		t.Fatalf("expected boot id: %q, got: %q", uuid1, id)
	}

	// insert the same boot id again, and should fail due to unique constraint
	err = InsertBootID(ctx, db, uuid1, first.Add(1*time.Second))
	if err == nil {
		t.Fatal("expected unique constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("expected unique constraint violation, got: %v", err)
	}

	// only one record, thus marked as non-reboot
	events, err := GetRebootEvents(ctx, db, time.Now().Add(-30*time.Second))
	if err != nil {
		t.Fatalf("failed to get reboot events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 reboot events, got: %d", len(events))
	}

	uuid2 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid2, first.Add(1*time.Second)); err != nil {
		t.Fatalf("failed to insert boot id: %v", err)
	}

	uuid3 := uuid.New().String()
	if err := InsertBootID(ctx, db, uuid3, first.Add(2*time.Second)); err != nil {
		t.Fatalf("failed to insert boot id: %v", err)
	}

	events, err = GetRebootEvents(ctx, db, time.Now().Add(-30*time.Second))
	if err != nil {
		t.Fatalf("failed to get reboot events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 reboot events, got: %d", len(events))
	}
	if events[0].BootID != uuid2 {
		t.Fatalf("expected boot id: %q, got: %q", uuid2, events[0].BootID)
	}
	if events[1].BootID != uuid3 {
		t.Fatalf("expected boot id: %q, got: %q", uuid3, events[1].BootID)
	}
}
