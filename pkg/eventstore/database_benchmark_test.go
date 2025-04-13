package eventstore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		ev := apiv1.Event{
			Time:    metav1.Time{Time: now.Add(time.Duration(i) * time.Minute)},
			Name:    "test",
			Type:    apiv1.EventTypeWarning,
			Message: "Test message with normal text",
			DeprecatedExtraInfo: map[string]string{
				"a": fmt.Sprintf("%d", i),
			},
			DeprecatedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeIgnoreNoActionRequired,
				},
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
