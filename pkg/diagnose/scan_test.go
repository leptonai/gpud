package diagnose

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestScan(t *testing.T) {
	// TODO: tailscale library has racey condition
	if os.Getenv("TEST_SCAN") != "true" {
		t.Skip("skipping scan")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := Scan(ctx, WithKMsgCheck(true)); err != nil {
		t.Logf("error scanning: %+v", err)
	}
}
