package powersupply

import (
	"context"
	"testing"
	"time"

	poller_config "github.com/leptonai/gpud/poller/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	component := New(
		ctx,
		Config{
			Query: poller_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
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
