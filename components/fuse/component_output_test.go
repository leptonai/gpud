package fuse

import (
	"context"
	"runtime"
	"testing"
	"time"

	events_db "github.com/leptonai/gpud/components/db"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	if err != nil {
		t.Fatalf("failed to create events store: %v", err)
	}
	defer eventsStore.Close()

	getFunc := CreateGet(Config{
		CongestedPercentAgainstThreshold:     90,
		MaxBackgroundPercentAgainstThreshold: 90,
	}, eventsStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = getFunc(ctx)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
}
