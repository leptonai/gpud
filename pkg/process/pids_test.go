package process

import (
	"context"
	"testing"
)

func TestCountProcessesByStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processes, err := CountProcessesByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("processes: %+v", processes)
}
