package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalInstanceTypeReader(t *testing.T) {
	plain := New("custom", nil, nil, nil, nil, nil)
	assert.False(t, SupportsInstanceType(plain))
	value, err := plain.(InstanceTypeDetector).InstanceType(t.Context())
	require.NoError(t, err)
	assert.Empty(t, value)
	assert.False(t, SupportsInstanceType(nil))

	failure := errors.New("metadata unavailable")
	for _, tc := range []struct {
		value string
		err   error
	}{{value: "p5.48xlarge"}, {err: failure}} {
		ctx, cancel := context.WithCancel(t.Context())
		d := NewIMDSWithRegion("aws", nil, nil, nil, nil, nil, nil, WithInstanceType(func(got context.Context) (string, error) {
			assert.Same(t, ctx, got)
			return tc.value, tc.err
		}))
		assert.True(t, SupportsInstanceType(d))
		value, err = d.(InstanceTypeDetector).InstanceType(ctx)
		assert.Equal(t, tc.value, value)
		assert.ErrorIs(t, err, tc.err)
		cancel()
	}
}
