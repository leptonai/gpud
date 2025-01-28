package diagnose

import (
	"context"
	"testing"
	"time"
)

func TestScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := Scan(ctx, WithDebug(true), WithNetcheck(false), WithDiskcheck(false)); err != nil {
		t.Fatalf("error scanning: %+v", err)
	}
}
