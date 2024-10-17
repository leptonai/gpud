package derp

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGetRegionLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latencies, err := MeasureLatencies(ctx, WithVerbose(true))
	if err != nil {
		t.Fatal(err)
	}
	latencies.RenderTable(os.Stdout)
}
