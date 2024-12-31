package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCreateSelectStatement(t *testing.T) {
	tests := []struct {
		name     string
		opts     []OpOption
		want     string
		wantArgs []any
		wantErr  bool
	}{
		{
			name: "no options",
			opts: nil,
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
ORDER BY %s DESC`,
				ColumnUnixSeconds,
				ColumnDeviceName,
				ColumnCongestedPercentAgainstThreshold,
				ColumnMaxBackgroundPercentAgainstThreshold,
				TableNameFUSEConnectionsEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with since unix seconds",
			opts: []OpOption{WithSince(time.Unix(1234, 0))},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
WHERE %s >= ?
ORDER BY %s DESC`,
				ColumnUnixSeconds,
				ColumnDeviceName,
				ColumnCongestedPercentAgainstThreshold,
				ColumnMaxBackgroundPercentAgainstThreshold,
				TableNameFUSEConnectionsEventHistory,
				ColumnUnixSeconds,
				ColumnUnixSeconds,
			),
			wantArgs: []any{int64(1234)},
			wantErr:  false,
		},
		{
			name: "with ascending order",
			opts: []OpOption{WithSortUnixSecondsAscendingOrder()},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
ORDER BY %s ASC`,
				ColumnUnixSeconds,
				ColumnDeviceName,
				ColumnCongestedPercentAgainstThreshold,
				ColumnMaxBackgroundPercentAgainstThreshold,
				TableNameFUSEConnectionsEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with limit",
			opts: []OpOption{WithLimit(10)},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
ORDER BY %s DESC
LIMIT 10`,
				ColumnUnixSeconds,
				ColumnDeviceName,
				ColumnCongestedPercentAgainstThreshold,
				ColumnMaxBackgroundPercentAgainstThreshold,
				TableNameFUSEConnectionsEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, args, err := createSelectStatementAndArgs(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("createSelectStatement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("createSelectStatement() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("createSelectStatement() args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestOpenMemory(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := CreateTableFUSEConnectionsEventHistory(ctx, db); err != nil {
		t.Fatal("failed to create table:", err)
	}
}

func TestInsertAndFindEvent(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	event := Event{
		UnixSeconds:                          time.Now().Unix(),
		DeviceName:                           "test_device",
		CongestedPercentAgainstThreshold:     75.5,
		MaxBackgroundPercentAgainstThreshold: 80.0,
	}

	// Test insertion
	err := InsertEvent(ctx, db, event)
	if err != nil {
		t.Errorf("InsertEvent failed: %v", err)
	}

	// Test finding the event
	foundEvent, err := FindEvent(ctx, db, event.UnixSeconds, event.DeviceName)
	if err != nil {
		t.Errorf("FindEvent failed: %v", err)
	}
	if foundEvent == nil {
		t.Error("expected to find event, but it wasn't found")
	}
	if foundEvent != nil && !reflect.DeepEqual(*foundEvent, event) {
		t.Errorf("found event doesn't match inserted event. got %+v, want %+v", foundEvent, event)
	}

	// Test finding non-existent event
	notFoundEvent, err := FindEvent(ctx, db, event.UnixSeconds+1, event.DeviceName)
	if err != nil {
		t.Errorf("FindEvent failed: %v", err)
	}
	if notFoundEvent != nil {
		t.Error("expected not to find event")
	}
}

func TestReadEvents_NoRows(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// test ReadEvents with empty table
	events, err := ReadEvents(ctx, db)
	if err != nil {
		t.Errorf("expected no error for empty table, got: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events for empty table, got: %v", events)
	}
}

func TestReadEvents(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseTime := time.Now().Unix()

	// Insert test events
	testEvents := []Event{
		{
			UnixSeconds:                          baseTime,
			DeviceName:                           "device1",
			CongestedPercentAgainstThreshold:     75.5,
			MaxBackgroundPercentAgainstThreshold: 80.0,
		},
		{
			UnixSeconds:                          baseTime + 1,
			DeviceName:                           "device2",
			CongestedPercentAgainstThreshold:     85.5,
			MaxBackgroundPercentAgainstThreshold: 90.0,
		},
		{
			UnixSeconds:                          baseTime + 2,
			DeviceName:                           "device3",
			CongestedPercentAgainstThreshold:     95.5,
			MaxBackgroundPercentAgainstThreshold: 100.0,
		},
	}

	for _, event := range testEvents {
		if err := InsertEvent(ctx, db, event); err != nil {
			t.Fatalf("failed to insert test event: %v", err)
		}
	}

	// test reading all events
	events, err := ReadEvents(ctx, db)
	if err != nil {
		t.Errorf("ReadEvents failed: %v", err)
	}
	if len(events) != len(testEvents) {
		t.Errorf("expected %d events, got %d", len(testEvents), len(events))
	}

	// test reading events with limit
	events, err = ReadEvents(ctx, db, WithLimit(2))
	if err != nil {
		t.Errorf("ReadEvents with limit failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events with limit, got %d", len(events))
	}

	// test reading events since specific time
	events, err = ReadEvents(ctx, db, WithSince(time.Unix(baseTime+1, 0)))
	if err != nil {
		t.Errorf("ReadEvents with since time failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events since baseTime+1, got %d", len(events))
	}

	// Test reading events with ascending order
	events, err = ReadEvents(ctx, db, WithSortUnixSecondsAscendingOrder())
	if err != nil {
		t.Errorf("ReadEvents with ascending order failed: %v", err)
	}
	if len(events) != 3 || events[0].UnixSeconds > events[len(events)-1].UnixSeconds {
		t.Error("Events not properly ordered in ascending order")
	}
}

func TestPurge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      []Event
		opts       []OpOption
		wantErr    bool
		wantPurged int
		wantCount  int
	}{
		{
			name: "delete events before timestamp",
			setup: []Event{
				{UnixSeconds: 1000, DeviceName: "device1"},
				{UnixSeconds: 2000, DeviceName: "device2"},
				{UnixSeconds: 3000, DeviceName: "device3"},
			},
			opts:       []OpOption{WithBefore(time.Unix(2500, 0))},
			wantPurged: 2,
			wantCount:  1,
		},
		{
			name: "delete all events",
			setup: []Event{
				{UnixSeconds: 1000, DeviceName: "device1"},
				{UnixSeconds: 2000, DeviceName: "device2"},
			},
			opts:       []OpOption{},
			wantPurged: 2,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, cleanup := setupTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Insert test data
			for _, event := range tt.setup {
				if err := InsertEvent(ctx, db, event); err != nil {
					t.Fatalf("failed to insert test event: %v", err)
				}
			}

			// perform deletion
			purged, err := Purge(ctx, db, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Purge() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if purged != tt.wantPurged {
				t.Errorf("Purge() purged = %v, want %v", purged, tt.wantPurged)
			}

			// verify results
			events, err := ReadEvents(ctx, db)
			if err != nil {
				t.Fatalf("failed to read events: %v", err)
			}

			if events == nil {
				if tt.wantCount != 0 {
					t.Errorf("expected %d events, got nil", tt.wantCount)
				}
			} else if len(events) != tt.wantCount {
				t.Errorf("expected %d events, got %d", tt.wantCount, len(events))
			}
		})
	}
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	tmpfile, err := os.CreateTemp("", "test-fuse-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpfile.Close()

	db, err := sqlite.Open(tmpfile.Name())
	if err != nil {
		os.Remove(tmpfile.Name())
		t.Fatalf("failed to open database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := CreateTableFUSEConnectionsEventHistory(ctx, db); err != nil {
		db.Close()
		os.Remove(tmpfile.Name())
		t.Fatalf("failed to create table: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpfile.Name())
	}
	return db, cleanup
}
