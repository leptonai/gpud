package infiniband

import (
	"context"
	"testing"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"

	"github.com/stretchr/testify/assert"
)

func TestComponentWithNoPoller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultPoller := nvidia_query.GetDefaultPoller()
	_, err := New(ctx, Config{})

	if defaultPoller != nil {
		// expects no error
		assert.NoError(t, err)
	} else {
		// expects error
		assert.Equal(t, err, nvidia_query.ErrDefaultPollerNotSet)
	}
}
