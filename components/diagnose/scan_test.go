package diagnose

import (
	"context"
	"testing"
	"time"
)

func TestScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := Scan(ctx); err != nil {
		t.Fatalf("error scanning: %+v", err)
	}
}
