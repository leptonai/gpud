package fd

import (
	"context"
	"testing"
	"time"

	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	component := New(
		ctx,
		Config{
			Query: query_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
				State: &query_config.State{
					DBRO: db,
					DBRW: db,
				},
			},
		},
	)

	time.Sleep(time.Second)

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
