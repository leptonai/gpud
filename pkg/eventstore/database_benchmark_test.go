package eventstore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
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

	bucket, err := database.Bucket("hello")
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
	t.Logf("db size: %s", humanize.IBytes(size)) // 361 M

	if err := sqlite.Compact(ctx, dbRW); err != nil {
		t.Fatalf("failed to compact db: %v", err)
	}

	size, err = sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size: %v", err)
	}
	t.Logf("db size: %s", humanize.IBytes(size)) // 341 MB
}
