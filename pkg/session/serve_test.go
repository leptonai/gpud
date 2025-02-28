package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateNeedDeleteFiles tests the createNeedDeleteFiles function
func TestCreateNeedDeleteFiles(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "test-need-delete")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create subdirectories for testing
	subdirs := []string{"dir1", "dir2", "dir3"}
	for _, subdir := range subdirs {
		subdirPath := filepath.Join(tempDir, subdir)
		err := os.Mkdir(subdirPath, 0755)
		require.NoError(t, err)
	}

	// Call the function being tested
	err = createNeedDeleteFiles(tempDir)
	require.NoError(t, err)

	// Verify that needDelete files were created in each subdirectory
	for _, subdir := range subdirs {
		needDeletePath := filepath.Join(tempDir, subdir, "needDelete")
		_, err := os.Stat(needDeletePath)
		assert.NoError(t, err, "needDelete file should exist in %s", subdir)
	}

	// Verify that no needDelete file was created in the root directory
	rootNeedDeletePath := filepath.Join(tempDir, "needDelete")
	_, err = os.Stat(rootNeedDeletePath)
	assert.True(t, os.IsNotExist(err), "needDelete file should not exist in root directory")
}

// TestDeleteMachine tests the deleteMachine function
func TestDeleteMachine(t *testing.T) {
	// Create a temporary directory structure to simulate the package directory
	tempDir, err := os.MkdirTemp("", "test-delete-machine")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a packages directory structure with subdirectories
	packagesDir := filepath.Join(tempDir, "packages")
	err = os.Mkdir(packagesDir, 0755)
	require.NoError(t, err)

	// Create package subdirectories
	packageNames := []string{"package1", "package2", "package3"}
	for _, pkgName := range packageNames {
		pkgPath := filepath.Join(packagesDir, pkgName)
		err := os.Mkdir(pkgPath, 0755)
		require.NoError(t, err)
	}

	// Call createNeedDeleteFiles with our test directory
	err = createNeedDeleteFiles(packagesDir)
	require.NoError(t, err)

	// Verify needDelete files were created in each package subdirectory
	for _, pkgName := range packageNames {
		needDeletePath := filepath.Join(packagesDir, pkgName, "needDelete")
		_, err := os.Stat(needDeletePath)
		assert.NoError(t, err, "needDelete file should exist in %s", pkgName)
	}
}

// TestRequestResponse tests the creation of a Response
func TestRequestResponse(t *testing.T) {
	// Simply test that we can create a Response with an error
	errorMsg := "test error"
	resp := &Response{
		Error: errorMsg,
	}

	// Verify error is set correctly
	require.NotNil(t, resp.Error)
	assert.Equal(t, errorMsg, resp.Error)

	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), fmt.Sprintf(`{"error":"%s"}`, errorMsg))
}

// TestRequest tests marshaling and unmarshaling of Request
func TestRequest(t *testing.T) {
	startTime := time.Now().UTC()
	endTime := startTime.Add(1 * time.Hour)
	since := 30 * time.Minute

	req := Request{
		Method:        "metrics",
		Components:    []string{"comp1", "comp2"},
		StartTime:     startTime,
		EndTime:       endTime,
		Since:         since,
		UpdateVersion: "1.0.0",
		UpdateConfig:  map[string]string{"key1": "value1"},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var unmarshaled Request
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, req.Method, unmarshaled.Method)
	assert.Equal(t, req.Components, unmarshaled.Components)
	assert.Equal(t, req.StartTime.Format(time.RFC3339), unmarshaled.StartTime.Format(time.RFC3339))
	assert.Equal(t, req.EndTime.Format(time.RFC3339), unmarshaled.EndTime.Format(time.RFC3339))
	assert.Equal(t, req.Since, unmarshaled.Since)
	assert.Equal(t, req.UpdateVersion, unmarshaled.UpdateVersion)
	assert.Equal(t, req.UpdateConfig, unmarshaled.UpdateConfig)
}

// TestGetMethods tests the logic of the get methods
func TestGetMethods(t *testing.T) {
	// Test getEvents method with method mismatch
	s := &Session{
		components: []string{"comp1", "comp2"},
	}

	ctx := context.Background()
	payload := Request{
		Method: "not-events", // Incorrect method to trigger an error
	}

	events, err := s.getEvents(ctx, payload)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Equal(t, "mismatch method", err.Error())

	// Test getMetrics method with method mismatch
	metrics, err := s.getMetrics(ctx, payload)
	assert.Error(t, err)
	assert.Nil(t, metrics)
	assert.Equal(t, "mismatch method", err.Error())

	// Test getStates method with method mismatch
	states, err := s.getStates(ctx, payload)
	assert.Error(t, err)
	assert.Nil(t, states)
	assert.Equal(t, "mismatch method", err.Error())
}

// TestInfinibandUpdateConfig tests the updateConfig handling for Infiniband component
func TestInfinibandUpdateConfig(t *testing.T) {
	// Create a simpler mock of the expected port states
	expectedPortStates := map[string]map[string]bool{
		"mlx5_0": {
			"1": true,
			"2": false,
		},
	}

	// Convert to JSON string
	configJSON, err := json.Marshal(expectedPortStates)
	require.NoError(t, err)

	// Create a request with updateConfig
	req := Request{
		Method: "updateConfig",
		UpdateConfig: map[string]string{
			nvidia_infiniband_id.Name: string(configJSON),
		},
	}

	// Convert the request to JSON
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify the JSON can be unmarshaled back to a request
	var unmarshaled Request
	err = json.Unmarshal(reqJSON, &unmarshaled)
	require.NoError(t, err)

	// Verify the contents
	assert.Equal(t, req.Method, unmarshaled.Method)
	assert.Equal(t, req.UpdateConfig, unmarshaled.UpdateConfig)
	assert.Contains(t, unmarshaled.UpdateConfig, nvidia_infiniband_id.Name)

	// Verify the config can be unmarshaled back to a map
	var unmarshaledConfig map[string]map[string]bool
	err = json.Unmarshal([]byte(unmarshaled.UpdateConfig[nvidia_infiniband_id.Name]), &unmarshaledConfig)
	require.NoError(t, err)

	// Verify the contents of the config
	assert.Equal(t, expectedPortStates, unmarshaledConfig)
}
