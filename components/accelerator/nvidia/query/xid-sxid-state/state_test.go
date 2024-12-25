package xidsxidstate

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
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
ORDER BY %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with since unix seconds",
			opts: []OpOption{WithSince(time.Unix(1234, 0))},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
WHERE %s >= ?
ORDER BY %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
				ColumnUnixSeconds,
			),
			wantArgs: []any{int64(1234)},
			wantErr:  false,
		},
		{
			name: "with ascending order",
			opts: []OpOption{WithSortUnixSecondsAscendingOrder()},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
ORDER BY %s ASC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with limit",
			opts: []OpOption{WithLimit(10)},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
ORDER BY %s DESC
LIMIT 10`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
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
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
WHERE %s >= ?
ORDER BY %s ASC
LIMIT 10`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
				ColumnUnixSeconds,
			),
			wantArgs: []any{int64(1234)},
			wantErr:  false,
		},
		{
			name: "with since unix seconds and event type",
			opts: []OpOption{WithSince(time.Unix(1234, 0)), WithEventType("xid")},
			want: fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s
WHERE %s >= ? AND %s = ?
ORDER BY %s DESC`,
				ColumnUnixSeconds,
				ColumnDataSource,
				ColumnEventType,
				ColumnEventID,
				ColumnEventDetails,
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
				ColumnEventType,
				ColumnUnixSeconds,
			),
			wantArgs: []any{int64(1234), "xid"},
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

	if err := CreateTableXidSXidEventHistory(ctx, db); err != nil {
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
		UnixSeconds:  time.Now().Unix(),
		DataSource:   "nvml",
		EventType:    "xid",
		EventID:      31,
		EventDetails: "GPU has fallen off the bus",
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
	eventDiffDetails.EventDetails = "Different details"
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
			UnixSeconds:  baseTime,
			DataSource:   "nvml",
			EventType:    "xid",
			EventID:      31,
			EventDetails: "First event",
		},
		{
			UnixSeconds:  baseTime + 1,
			DataSource:   "dmesg",
			EventType:    "sxid",
			EventID:      32,
			EventDetails: "Second event",
		},
		{
			UnixSeconds:  baseTime + 2,
			DataSource:   "nvml",
			EventType:    "xid",
			EventID:      33,
			EventDetails: "Third event",
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

func TestCreateDeleteStatementAndArgs(t *testing.T) {
	tests := []struct {
		name          string
		opts          []OpOption
		wantStatement string
		wantArgs      []any
		wantErr       bool
	}{
		{
			name: "no options",
			opts: []OpOption{},
			wantStatement: fmt.Sprintf("DELETE FROM %s",
				TableNameXidSXidEventHistory,
			),
			wantArgs: nil,
			wantErr:  false,
		},
		{
			name: "with before unix seconds and limit",
			opts: []OpOption{
				WithBefore(time.Unix(1234, 0)),
				WithLimit(10),
			},
			wantStatement: fmt.Sprintf("DELETE FROM %s WHERE %s < ?",
				TableNameXidSXidEventHistory,
				ColumnUnixSeconds,
			),
			wantArgs: []any{int64(1234)},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatement, gotArgs, err := createDeleteStatementAndArgs(tt.opts...)

			if tt.wantErr {
				if err == nil {
					t.Error("createDeleteStatementAndArgs() error = nil, wantErr = true")
				}
				return
			}

			if err != nil {
				t.Errorf("createDeleteStatementAndArgs() error = %v, wantErr = false", err)
				return
			}

			if gotStatement != tt.wantStatement {
				t.Errorf("createDeleteStatementAndArgs() statement = %v, want %v", gotStatement, tt.wantStatement)
			}

			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("createDeleteStatementAndArgs() args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
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
		validate   func(*testing.T, []Event)
	}{
		{
			name: "delete events before timestamp",
			setup: []Event{
				{UnixSeconds: 1000, DataSource: "nvml", EventType: "xid", EventID: 1, EventDetails: "detail1"},
				{UnixSeconds: 2000, DataSource: "nvml", EventType: "xid", EventID: 2, EventDetails: "detail2"},
				{UnixSeconds: 3000, DataSource: "nvml", EventType: "xid", EventID: 3, EventDetails: "detail3"},
			},
			opts:       []OpOption{WithBefore(time.Unix(2500, 0))},
			wantPurged: 2,
			wantCount:  1,
			validate: func(t *testing.T, events []Event) {
				if len(events) == 0 || events[0].UnixSeconds != 3000 {
					t.Errorf("expected event with timestamp 3000, got %+v", events)
				}
			},
		},
		{
			name: "delete all events",
			setup: []Event{
				{UnixSeconds: 1000, DataSource: "nvml", EventType: "xid", EventID: 1, EventDetails: "detail1"},
				{UnixSeconds: 2000, DataSource: "nvml", EventType: "xid", EventID: 2, EventDetails: "detail2"},
			},
			opts:       []OpOption{},
			wantPurged: 2,
			wantCount:  0,
		},
		{
			name: "delete events with large dataset",
			setup: func() []Event {
				events := make([]Event, 100)
				baseTime := time.Now().Unix()
				for i := 0; i < 100; i++ {
					events[i] = Event{
						UnixSeconds:  baseTime + int64(i*60), // Events 1 minute apart
						DataSource:   "nvml",
						EventType:    "xid",
						EventID:      int64(i + 1),
						EventDetails: fmt.Sprintf("detail%d", i+1),
					}
				}
				return events
			}(),
			opts:       []OpOption{WithBefore(time.Now().Add(30 * time.Minute))},
			wantPurged: 30,
			wantCount:  70,
			validate: func(t *testing.T, events []Event) {
				if len(events) != 70 {
					t.Errorf("expected 70 events, got %d", len(events))
				}
				cutoff := time.Now().Add(30 * time.Minute).Unix()
				for _, e := range events {
					if e.UnixSeconds < cutoff {
						t.Errorf("found event with timestamp %d, which is before cutoff %d",
							e.UnixSeconds, cutoff)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// setup fresh database for each test
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
				t.Errorf("DeleteEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if purged != tt.wantPurged {
				t.Errorf("DeleteEvents() purged = %v, want %v", purged, tt.wantPurged)
			}

			// verify results
			events, err := ReadEvents(ctx, db)
			if err != nil {
				t.Fatalf("failed to read events: %v", err)
			}

			if len(events) != tt.wantCount {
				t.Errorf("expected %d events, got %d", tt.wantCount, len(events))
			}

			if tt.validate != nil {
				tt.validate(t, events)
			}
		})
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

	if err := CreateTableXidSXidEventHistory(ctx, db); err != nil {
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
