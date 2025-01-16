package os

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/pkg/sqlite"
	poller_config "github.com/leptonai/gpud/poller/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer dbRO.Close()

	if err := state.CreateTableBootIDs(ctx, dbRW); err != nil {
		t.Fatalf("failed to create boot ids table: %v", err)
	}

	component := New(
		ctx,
		Config{
			PollerConfig: poller_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
				State: &poller_config.State{
					DBRW: dbRW,
					DBRO: dbRO,
				},
			},
		},
	)

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
