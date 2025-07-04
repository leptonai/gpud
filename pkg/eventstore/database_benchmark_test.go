package eventstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// BENCHMARK=true go test -v -run=TestSimulatedEvents -timeout=10m
func TestSimulatedEvents(t *testing.T) {
	if os.Getenv("BENCHMARK") != "true" {
		t.Skip("skipping benchmark test")
	}

	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	database, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	tableName := defaultTableName("hello")
	bucket, err := database.Bucket(tableName)
	assert.NoError(t, err)
	defer bucket.Close()
	daysToIngest := 3
	eventsN := daysToIngest * 24 * 60 * 60

	now := time.Now()
	for i := 0; i < eventsN; i++ {
		ev := Event{
			Time:    now.Add(time.Duration(i) * time.Minute),
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "Test message with normal text",
			ExtraInfo: map[string]string{
				"a": fmt.Sprintf("%d", i),
			},
		}
		if err := bucket.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}
	t.Logf("ingested %d events", eventsN)

	size, err := sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size: %v", err)
	}
	t.Logf("db size: %s", humanize.Bytes(size)) // 361 M

	if err := sqlite.Compact(ctx, dbRW); err != nil {
		t.Fatalf("failed to compact db: %v", err)
	}

	size, err = sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size: %v", err)
	}
	t.Logf("db size: %s", humanize.Bytes(size)) // 341 MB
}

// go test -v -run=^$ -bench=BenchmarkEventStore_GetEvents_Filtering -count=1 -timeout=10m -benchmem > /tmp/baseline.txt
// USE_NEW=true go test -v -run=^$ -bench=BenchmarkEventStore_GetEvents_Filtering -count=1 -timeout=10m -benchmem > /tmp/new.txt
// benchstat /tmp/baseline.txt /tmp/new.txt
func BenchmarkEventStore_GetEvents_Filtering(b *testing.B) {
	db, cleanup := setupBenchmarkEventStore(b)
	defer cleanup()

	// Populate with 5 different event names, 500 events each
	populateEventsWithMultipleNames(b, db, 500)

	bucket, err := db.Bucket("benchmark_bucket", WithDisablePurge())
	if err != nil {
		b.Fatalf("failed to create bucket: %v", err)
	}
	defer bucket.Close()

	ctx := context.Background()
	since := time.Now().UTC().Add(-1 * time.Hour)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if os.Getenv("USE_NEW") != "true" {
			// Method 1: Get all events and filter by event name using if-statement
			allEvents, err := bucket.Get(ctx, since)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
			var filteredEvents Events
			for _, event := range allEvents {
				if event.Name != "event1" {
					filteredEvents = append(filteredEvents, event)
				}
			}
			_ = filteredEvents
		} else {
			// Method 2: Use SQL-level filtering to exclude specific event names
			// This should be more efficient as filtering happens at the database level
			filteredEvents, err := bucket.Get(ctx, since, WithEventNamesToExclude("event1"))
			if err != nil {
				b.Fatalf("Get with exclude failed: %v", err)
			}
			_ = filteredEvents
		}
	}
}

func setupBenchmarkEventStore(b *testing.B) (Store, func()) {
	// Create temporary database file for benchmark
	tmpf, err := os.CreateTemp(os.TempDir(), "benchmark-eventstore-sqlite")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}

	dbRW, err := sql.Open("sqlite3", tmpf.Name())
	if err != nil {
		b.Fatalf("failed to open database: %v", err)
	}

	dbRO, err := sql.Open("sqlite3", tmpf.Name())
	if err != nil {
		b.Fatalf("failed to open database: %v", err)
	}

	database, err := New(dbRW, dbRO, 0)
	if err != nil {
		b.Fatalf("failed to create eventstore: %v", err)
	}

	cleanup := func() {
		_ = dbRW.Close()
		_ = dbRO.Close()
		_ = os.Remove(tmpf.Name())
	}

	return database, cleanup
}

func populateEventsWithMultipleNames(b *testing.B, db Store, numEventsPerName int) {
	eventNames := []string{"event1", "event2", "event3", "event4", "event5"}

	bucket, err := db.Bucket("benchmark_bucket", WithDisablePurge())
	if err != nil {
		b.Fatalf("failed to create bucket: %v", err)
	}
	defer bucket.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	for i, eventName := range eventNames {
		for j := 0; j < numEventsPerName; j++ {
			ev := Event{
				Time:    now.Add(time.Duration(i*numEventsPerName+j) * time.Second),
				Name:    eventName,
				Type:    string(apiv1.EventTypeInfo),
				Message: fmt.Sprintf("Test message for %s event %d", eventName, j),
				ExtraInfo: map[string]string{
					"event_type": eventName,
					"index":      fmt.Sprintf("%d", j),
				},
			}
			if err := bucket.Insert(ctx, ev); err != nil {
				b.Fatalf("failed to insert event: %v", err)
			}
		}
	}
}
