package info

import (
	"context"
	"testing"
)

func TestComponent(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{"a": "b"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	states, err := component.States(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	t.Logf("states: %+v", states)
}
