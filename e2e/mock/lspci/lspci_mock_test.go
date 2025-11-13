package lspci

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2emock "github.com/leptonai/gpud/e2e/mock"
	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
)

func TestMock(t *testing.T) {
	// Set up the mock
	err := Mock(NormalOutput)
	require.NoError(t, err)

	// Verify the mock file exists and has the correct permissions
	dir, err := e2emock.GetMockDir()
	require.NoError(t, err)
	mockFile := filepath.Join(dir, "lspci")

	// Wait a short time to ensure the mock file is fully written
	time.Sleep(100 * time.Millisecond)

	// Verify the mock file exists
	_, err = os.Stat(mockFile)
	require.NoError(t, err, "Mock file should exist")

	// Verify PATH includes the mock directory
	path := os.Getenv("PATH")
	require.Contains(t, path, dir, "PATH should include mock directory")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Retry the ListNVIDIAPCIs call a few times if it fails
	var deviceNames []string
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		deviceNames, err = nvidiapci.ListPCIGPUs(ctx)
		if err == nil && len(deviceNames) > 0 {
			break
		}
		lastErr = err
		t.Logf("Retry %d: ListNVIDIAPCIs failed or returned empty. Error: %v", i+1, err)
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("ListNVIDIAPCIs failed after %d retries: %v", maxRetries, lastErr)
	}

	require.NotEmpty(t, deviceNames, "Device names should not be empty")
	assert.Contains(t, deviceNames, "06:00.0 3D controller [0302]: NVIDIA Corporation GA102GL [A10] [10de:2236] (rev a1)",
		"Expected NVIDIA device not found in output")
}
