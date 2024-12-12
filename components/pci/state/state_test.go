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
ORDER BY %s DESC, %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnReasons,
				TableNamePCIEvents,
				ColumnUnixSeconds,
				ColumnDataSource,
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
ORDER BY %s DESC, %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnReasons,
				TableNamePCIEvents,
				ColumnUnixSeconds,
				ColumnUnixSeconds,
				ColumnDataSource,
			),
			wantArgs: []any{int64(1234)},
			wantErr:  false,
		},
		{
			name: "with ascending order",
			opts: []OpOption{WithSortUnixSecondsAscendingOrder()},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
ORDER BY %s ASC, %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnReasons,
				TableNamePCIEvents,
				ColumnUnixSeconds,
				ColumnDataSource,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with limit",
			opts: []OpOption{WithLimit(10)},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
ORDER BY %s DESC, %s DESC
LIMIT 10`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnReasons,
				TableNamePCIEvents,
				ColumnUnixSeconds,
				ColumnDataSource,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with all options",
			opts: []OpOption{
				WithSince(time.Unix(1234, 0)),
				WithSortUnixSecondsAscendingOrder(),
				WithLimit(10),
			},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s
WHERE %s >= ?
ORDER BY %s ASC, %s DESC
LIMIT 10`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnReasons,
				TableNamePCIEvents,
				ColumnUnixSeconds,
				ColumnUnixSeconds,
				ColumnDataSource,
			),
			wantArgs: []any{int64(1234)},
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

	if err := CreateTable(ctx, db); err != nil {
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
		UnixSeconds: time.Now().Unix(),
		DataSource:  "lspci -vv",
		EventType:   "devices_with_acs",
		Reasons:     []string{"GPU has fallen off the bus"},
	}

	// Test insertion
	err := InsertEvent(ctx, db, event)
	if err != nil {
		t.Errorf("InsertEvent failed: %v", err)
	}

	// Test finding the event
	found, err := FindEvent(ctx, db, event)
	if err != nil {
		t.Errorf("FindEvent failed: %v", err)
	}
	if !found {
		t.Error("expected to find event, but it wasn't found")
	}

	// Test finding event with different details
	eventDiffDetails := event
	eventDiffDetails.Reasons = []string{"Different details"}
	found, err = FindEvent(ctx, db, eventDiffDetails)
	if err != nil {
		t.Errorf("FindEvent with different details failed: %v", err)
	}
	if found {
		t.Error("expected not to find event with different details")
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
			UnixSeconds: baseTime,
			DataSource:  "lspci -vv",
			EventType:   "devices_with_acs",
			Reasons:     []string{"First event"},
		},
		{
			UnixSeconds: baseTime + 1,
			DataSource:  "nvidia-smi",
			EventType:   "devices_with_acs",
			Reasons:     []string{"Second event"},
		},
		{
			UnixSeconds: baseTime + 2,
			DataSource:  "lspci -vv",
			EventType:   "devices_with_acs",
			Reasons:     []string{"Third event"},
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

	t.Logf("searching for events since: %d", baseTime+1)
	for _, e := range events {
		t.Logf("Found event with timestamp: %d", e.UnixSeconds)
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

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	tmpfile, err := os.CreateTemp("", "test-nvidia-*.db")
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

	if err := CreateTable(ctx, db); err != nil {
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
