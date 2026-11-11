package all

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
)

func TestInstanceTypeDetection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		detector := providers.NewIMDSWithRegion("aws", func(context.Context) (string, error) { return "aws", nil }, nil, nil, nil, nil, nil,
			providers.WithInstanceType(func(context.Context) (string, error) {
				calls++
				if calls == 1 {
					return "unknown", nil
				}
				return " p5.48xlarge ", nil
			}))
		assert.Equal(t, "p5.48xlarge", detectInstanceType(t.Context(), []providers.Detector{detector}))
		assert.Equal(t, 2, calls)
		unsupported := &legacyDetector{name: "nebius", provider: "nebius"}
		assert.Empty(t, detectInstanceType(t.Context(), []providers.Detector{unsupported, detector}))
		assert.Equal(t, 2, calls, "do not misattribute an unsupported provider to AWS")
	})
}

func TestInstanceTypeTimeoutIsOptional(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		detector := providers.NewIMDSWithRegion("aws", nil, nil, nil, nil, nil, nil,
			providers.WithInstanceType(func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }))
		start := time.Now()
		assert.Empty(t, fetchInstanceType(t.Context(), detector))
		require.LessOrEqual(t, time.Since(start), 5*time.Second)
	})
}

func TestDetectPreservesIdentityWhenInstanceTypeUnavailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := All
		defer func() { All = original }()
		read := func(value string) func(context.Context) (string, error) {
			return func(context.Context) (string, error) { return value, nil }
		}
		All = []providers.Detector{providers.NewIMDSWithRegion("aws", read("aws"), nil, nil, read("us-east-1"), nil, read("i-123"),
			providers.WithInstanceType(func(ctx context.Context) (string, error) {
				<-ctx.Done()
				return "", ctx.Err()
			}))}
		info, err := Detect(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Equal(t, "i-123", info.InstanceID)
		assert.Equal(t, "us-east-1", info.Region)
		assert.Empty(t, info.InstanceType)
	})
}
