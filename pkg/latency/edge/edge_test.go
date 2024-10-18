package edge

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMeasure(t *testing.T) {
	t.Skip("skipping test")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latencies, err := Measure(ctx, WithVerbose(true))
	if err != nil {
		t.Fatal(err)
	}
	latencies.RenderTable(os.Stdout)
}
