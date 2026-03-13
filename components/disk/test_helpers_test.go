package disk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func mustCheckResult(t *testing.T, result components.CheckResult) *checkResult {
	t.Helper()

	cr, ok := result.(*checkResult)
	require.True(t, ok)
	return cr
}

func mustComponent(t *testing.T, comp components.Component) *component {
	t.Helper()

	c, ok := comp.(*component)
	require.True(t, ok)
	return c
}

func mustMockEventBucket(t *testing.T, bucket any) *mockEventBucket {
	t.Helper()

	mb, ok := bucket.(*mockEventBucket)
	require.True(t, ok)
	return mb
}
