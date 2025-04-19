package scan

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestScan(t *testing.T) {
	if os.Getenv("TEST_GPUD_SCAN") != "true" {
		t.Skip("skipping scan test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := Scan(ctx); err != nil {
		t.Logf("error scanning: %+v", err)
	}
}
