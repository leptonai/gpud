package os

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentStates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	component, err := New(
		ctx,
		Config{
			Query: query_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
				State: &query_config.State{
					DBRW: dbRW,
					DBRO: dbRO,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}
	defer component.Close()

	time.Sleep(2 * time.Second)

	states, err := component.States(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	t.Logf("states: %+v", states)

	parsedOutput, err := ParseStatesToOutput(states...)
	if err != nil {
		t.Fatalf("failed to parse states: %v", err)
	}
	t.Logf("parsed output: %+v", parsedOutput)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	c, err := New(
		ctx,
		Config{
			Query: query_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
				State: &query_config.State{
					DBRW: dbRW,
					DBRO: dbRO,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}
	defer c.Close()

	now := time.Now()
	eventsN := 20

	events, err := c.Events(ctx, now.Add(-time.Duration(eventsN+5)*time.Second))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}

	cc, _ := c.(*component)

	for i := 0; i < eventsN; i++ {
		if err := cc.eventsStore.Insert(ctx, components.Event{
			Time:    metav1.Time{Time: now.Add(-time.Duration(i) * time.Second)},
			Name:    "reboot",
			Type:    common.EventTypeWarning,
			Message: fmt.Sprintf("boot-%d", i),
		}); err != nil {
			t.Fatalf("failed to insert boot id: %v", err)
		}
	}

	events, err = c.Events(ctx, now.Add(-time.Duration(eventsN+5)*time.Second))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) != eventsN {
		t.Fatalf("expected %d events, got %d", eventsN, len(events))
	}
}
