package info

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{"a": "b"}, nil, prometheus.DefaultGatherer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	states, err := component.States(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	t.Logf("states: %+v", states)
}
