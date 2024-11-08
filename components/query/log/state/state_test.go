package state_test

import (
	"context"
	"database/sql"
	"math/rand"
	"os"
	"testing"
	"time"

	logstate "github.com/leptonai/gpud/components/query/log/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestOpenMemory(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := logstate.CreateTable(ctx, db); err != nil {
		t.Fatalf("failed to create log table: %v", err)
	}

	offset := rand.Int63n(10000)
	whence := rand.Int63n(100)
	if err := logstate.Insert(ctx, db, "test-file", offset, whence); err != nil {
		t.Fatalf("failed to insert log: %v", err)
	}

	offset2, whence2, err := logstate.Get(ctx, db, "test-file")
	if err != nil {
		t.Fatalf("failed to get log: %v", err)
	}
	if offset != offset2 || whence != whence2 {
		t.Fatalf("log mismatch: %d %d %d %d", offset, whence, offset2, whence2)
	}

	if _, _, err := logstate.Get(ctx, db, "invalid"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestOpen(t *testing.T) {
	f, err := os.CreateTemp(os.TempDir(), "test-db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	db, err := sqlite.Open(f.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := logstate.CreateTable(ctx, db); err != nil {
		t.Fatalf("failed to create log table: %v", err)
	}

	offset := rand.Int63n(10000)
	whence := rand.Int63n(100)
	if err := logstate.Insert(ctx, db, "test-file", offset, whence); err != nil {
		t.Fatalf("failed to insert log: %v", err)
	}
	if err := logstate.Insert(ctx, db, "test-file", offset+1, whence); err != nil {
		t.Fatalf("failed to insert log: %v", err)
	}

	offset2, whence2, err := logstate.Get(ctx, db, "test-file")
	if err != nil {
		t.Fatalf("failed to get log: %v", err)
	}
	if offset+1 != offset2 || whence != whence2 {
		t.Fatalf("log mismatch: %d %d %d %d", offset+1, whence, offset2, whence2)
	}

	if _, _, err := logstate.Get(ctx, db, "invalid"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	db.Close()

	db, err = sqlite.Open(f.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	offset3, whence3, err := logstate.Get(ctx, db, "test-file")
	if err != nil {
		t.Fatalf("failed to get log: %v", err)
	}
	if offset+1 != offset3 || whence != whence3 {
		t.Fatalf("log mismatch: %d %d %d %d", offset+1, whence, offset3, whence3)
	}
}
