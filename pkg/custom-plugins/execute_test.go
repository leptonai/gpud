package customplugins

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/stretchr/testify/assert"
)

func TestComponentSpecsExecuteInOrder(t *testing.T) {
	testFile := filepath.Join("testdata", "plugins.plaintext.2.regex.yaml")
	specs, err := LoadSpecs(testFile)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	results, err := specs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx})
	assert.NoError(t, err)
	assert.Equal(t, 5, len(results))

	for _, rs := range results {
		t.Logf("%q: %q", rs.ComponentName(), rs.Summary())
	}
}

// TestExecuteInOrderWithCanceledContext tests the behavior when the context is canceled
func TestExecuteInOrderWithCanceledContext(t *testing.T) {
	testFile := filepath.Join("testdata", "plugins.plaintext.2.regex.yaml")
	specs, err := LoadSpecs(testFile)
	assert.NoError(t, err)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Since we're canceling the context, but the ExecuteInOrder function doesn't currently check
	// for context cancellation between plugin executions, this should still succeed
	// This test is added for completeness and to document the current behavior
	results, err := specs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx})
	assert.NoError(t, err)
	assert.Equal(t, 5, len(results))
}
