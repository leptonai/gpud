package state

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestAPIVersion(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := CreateTableAPIVersion(ctx, db); err != nil {
		t.Fatalf("failed to create api version table: %v", err)
	}

	ver, err := UpdateAPIVersionIfNotExists(ctx, db, "v1")
	if err != nil {
		t.Fatalf("failed to update api version: %v", err)
	}
	if ver != "v1" {
		t.Fatalf("api version mismatch: %s != v1", ver)
	}

	ver, err = ReadAPIVersion(ctx, db)
	if err != nil {
		t.Fatalf("failed to read api version: %v", err)
	}
	if ver != "v1" {
		t.Fatalf("api version mismatch: %s != v1", ver)
	}
}
