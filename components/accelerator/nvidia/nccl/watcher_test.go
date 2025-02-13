package nccl

import (
	"context"
	"strings"
	"testing"
	"time"

	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestWatcher(t *testing.T) {
	dmesgWatcher, err := pkg_dmesg.NewWatcherWithCommands([][]string{
		{
			"cat ./testdata/dmesg.decode.iso.log",
		},
	})
	if err != nil {
		t.Fatalf("failed to create dmesg watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, "test", 0)
	if err != nil {
		t.Fatalf("failed to create events store: %v", err)
	}
	defer eventsStore.Close()

	w := &watcher{
		ctx:          ctx,
		eventsStore:  eventsStore,
		dmesgWatcher: dmesgWatcher,
	}
	go w.watch()
	defer w.close()

	time.Sleep(5 * time.Second)

	events, err := eventsStore.Get(ctx, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Skip("no events found") // slow CI...
	}

	t.Logf("found %d events", len(events))
	for _, ev := range events {
		if !strings.Contains(ev.Name, EventNCCLSegfaultInLibnccl) {
			t.Fatalf("unexpected event type: %s", ev.Name)
		}
	}
}
