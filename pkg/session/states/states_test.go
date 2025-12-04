package states

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCreateTable(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	err := CreateTable(ctx, dbRW)
	if err != nil {
		t.Fatalf("failed to create session_states table: %v", err)
	}

	// Verify table was created by inserting a record
	err = Insert(ctx, dbRW, time.Now().Unix(), true, "test message")
	if err != nil {
		t.Fatalf("failed to insert into session_states table: %v", err)
	}

	// Verify we can read from the table
	status, err := ReadLast(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read from session_states table: %v", err)
	}
	if status == nil {
		t.Fatal("expected status to be non-nil")
	}
}

func TestInsert(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		timestamp int64
		success   bool
		message   string
	}{
		{
			name:      "success entry",
			timestamp: time.Now().Unix(),
			success:   true,
			message:   "Session connected successfully",
		},
		{
			name:      "failure entry",
			timestamp: time.Now().Unix() + 1,
			success:   false,
			message:   "HTTP 403: Forbidden",
		},
		{
			name:      "empty message",
			timestamp: time.Now().Unix() + 2,
			success:   true,
			message:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh database for each subtest
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			err := CreateTable(ctx, dbRW)
			if err != nil {
				t.Fatalf("failed to create session_states table: %v", err)
			}

			err = Insert(ctx, dbRW, tt.timestamp, tt.success, tt.message)
			if err != nil {
				t.Fatalf("failed to insert login status: %v", err)
			}

			status, err := ReadLast(ctx, dbRO)
			if err != nil {
				t.Fatalf("failed to read login status: %v", err)
			}
			if status == nil {
				t.Fatal("expected status to be non-nil")
			}
			if status.Timestamp != tt.timestamp {
				t.Errorf("expected timestamp %d, got %d", tt.timestamp, status.Timestamp)
			}
			if status.Success != tt.success {
				t.Errorf("expected success %v, got %v", tt.success, status.Success)
			}
			if status.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, status.Message)
			}
		})
	}
}

func TestInsertPrunesOldEntries(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	err := CreateTable(ctx, dbRW)
	if err != nil {
		t.Fatalf("failed to create session_states table: %v", err)
	}

	// Insert 15 entries
	baseTime := time.Now().Unix()
	for i := 0; i < 15; i++ {
		err := Insert(ctx, dbRW, baseTime+int64(i), true, "test message")
		if err != nil {
			t.Fatalf("failed to insert login status: %v", err)
		}
	}

	// Count remaining entries (Insert cleans up automatically)
	var count int
	err = dbRO.QueryRowContext(ctx, "SELECT COUNT(*) FROM session_states").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count entries: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 entries after cleanup, got %d", count)
	}

	// Verify the latest entry is still present
	status, err := ReadLast(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read latest login status: %v", err)
	}
	if status == nil {
		t.Fatal("expected status to be non-nil")
	}
	expectedLatestTimestamp := baseTime + 14
	if status.Timestamp != expectedLatestTimestamp {
		t.Errorf("expected latest timestamp %d, got %d", expectedLatestTimestamp, status.Timestamp)
	}
}

func TestReadLast(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	err := CreateTable(ctx, dbRW)
	if err != nil {
		t.Fatalf("failed to create session_states table: %v", err)
	}

	// Test reading from empty table
	status, err := ReadLast(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read from empty table: %v", err)
	}
	if status != nil {
		t.Error("expected nil status from empty table")
	}

	// Insert multiple entries
	baseTime := time.Now().Unix()
	for i := 0; i < 5; i++ {
		err := Insert(ctx, dbRW, baseTime+int64(i), i%2 == 0, "message")
		if err != nil {
			t.Fatalf("failed to insert login status: %v", err)
		}
	}

	// Should return the latest entry
	status, err = ReadLast(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read latest login status: %v", err)
	}
	if status == nil {
		t.Fatal("expected status to be non-nil")
	}
	expectedLatestTimestamp := baseTime + 4
	if status.Timestamp != expectedLatestTimestamp {
		t.Errorf("expected latest timestamp %d, got %d", expectedLatestTimestamp, status.Timestamp)
	}
}

func TestHasAnyFailures(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	err := CreateTable(ctx, dbRW)
	if err != nil {
		t.Fatalf("failed to create session_states table: %v", err)
	}

	// Empty table should have no failures
	hasFailures, err := HasAnyFailures(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to check for failures: %v", err)
	}
	if hasFailures {
		t.Error("expected no failures in empty table")
	}

	// Insert only success entries
	baseTime := time.Now().Unix()
	for i := 0; i < 3; i++ {
		err := Insert(ctx, dbRW, baseTime+int64(i), true, "success message")
		if err != nil {
			t.Fatalf("failed to insert login status: %v", err)
		}
	}

	hasFailures, err = HasAnyFailures(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to check for failures: %v", err)
	}
	if hasFailures {
		t.Error("expected no failures with only success entries")
	}

	// Insert a failure entry
	err = Insert(ctx, dbRW, baseTime+3, false, "failure message")
	if err != nil {
		t.Fatalf("failed to insert login status: %v", err)
	}

	hasFailures, err = HasAnyFailures(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to check for failures: %v", err)
	}
	if !hasFailures {
		t.Error("expected to detect failure entry")
	}
}

func TestInsertPreservesLatestEntries(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	err := CreateTable(ctx, dbRW)
	if err != nil {
		t.Fatalf("failed to create session_states table: %v", err)
	}

	// Insert 15 entries with different timestamps
	baseTime := time.Now().Unix()
	for i := 0; i < 15; i++ {
		success := i%3 != 0 // Mix of success and failure
		err := Insert(ctx, dbRW, baseTime+int64(i), success, "message")
		if err != nil {
			t.Fatalf("failed to insert login status: %v", err)
		}
	}

	// Read all remaining entries (Insert cleans up automatically)
	rows, err := dbRO.QueryContext(ctx, "SELECT timestamp FROM session_states ORDER BY timestamp ASC")
	if err != nil {
		t.Fatalf("failed to query entries: %v", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var timestamps []int64
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			t.Fatalf("failed to scan timestamp: %v", err)
		}
		timestamps = append(timestamps, ts)
	}

	if len(timestamps) != 10 {
		t.Fatalf("expected 10 timestamps, got %d", len(timestamps))
	}

	// Verify we have the latest 10 entries
	for i, ts := range timestamps {
		expected := baseTime + int64(5+i) // Should have entries 5-14
		if ts != expected {
			t.Errorf("expected timestamp %d at position %d, got %d", expected, i, ts)
		}
	}
}
