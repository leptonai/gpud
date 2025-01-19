package fd

import (
	"context"
	"testing"
	"time"

	poller_config "github.com/leptonai/gpud/pkg/poller/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	component, err := New(
		ctx,
		Config{
			PollerConfig: poller_config.Config{
				Interval: metav1.Duration{Duration: 5 * time.Second},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

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
