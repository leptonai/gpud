package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/pkg/sqlite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
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

	tableName := CreateDefaultTableName("hello")
	store, err := NewStore(dbRW, dbRO, tableName, 0)
	assert.NoError(t, err)
	defer store.Close()
	daysToIngest := 3
	eventsN := daysToIngest * 24 * 60 * 60

	now := time.Now()
	for i := 0; i < eventsN; i++ {
		ev := components.Event{
			Time:    metav1.Time{Time: now.Add(time.Duration(i) * time.Minute)},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "Test message with normal text",
			ExtraInfo: map[string]string{
				"a": fmt.Sprintf("%d", i),
			},
			SuggestedActions: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{
					common.RepairActionTypeIgnoreNoActionRequired,
				},
			},
		}
		if err := store.Insert(ctx, ev); err != nil {
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
