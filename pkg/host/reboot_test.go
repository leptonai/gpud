package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRunReboot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with a non-root user
	err := runReboot(ctx, "echo reboot")
	assert.NoError(t, err)
}

func TestBootTime(t *testing.T) {
	original := currentBootTimeUnixSeconds
	defer func() { currentBootTimeUnixSeconds = original }()

	currentBootTimeUnixSeconds = 0
	assert.True(t, BootTime().IsZero())

	expected := time.Unix(1_700_000_000, 0).UTC()
	currentBootTimeUnixSeconds = uint64(expected.Unix())
	assert.True(t, BootTime().Equal(expected))
}
