package state

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/dustin/go-humanize"
)

// go test -v -run=TestSimulatedEvents -timeout=10m
func TestSimulatedEvents(t *testing.T) {
	if os.Getenv("BENCHMARK") != "true" {
		t.Skip("skipping benchmark test")
	}

	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	daysToIngest := 3
	minutesToIngest := daysToIngest * 24 * 60 * 60
	componentsN := 10
	eventsN := minutesToIngest * componentsN

	now := time.Now()

	for i := 0; i < componentsN; i++ {
		component := fmt.Sprintf("component_%d", i)
		t.Logf("ingesting %d events for component %s", minutesToIngest, component)

		for j := 0; j < minutesToIngest; j++ {
			ev := Event{
				Timestamp:    now.Add(time.Duration(j) * time.Minute).Unix(),
				EventType:    component,
				DataSource:   "test_data_source",
				Target:       "test_target",
				EventDetails: "test_event_details",
			}
			if err := InsertEvent(ctx, dbRW, ev); err != nil {
				t.Fatalf("failed to insert event: %v", err)
			}
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
