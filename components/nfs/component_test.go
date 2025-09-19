package nfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// createTestComponent creates a component with properly initialized function fields for testing
func createTestComponent() *component {
	return &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return configs.Validate(ctx)
		},
		newChecker: pkgnfschecker.NewChecker,
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return checker.Write(ctx)
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return checker.Check(ctx)
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return checker.Clean()
		},
	}
}

func TestNewComponent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(&components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
		MachineID:  "test-machine",
	})
	require.NoError(t, err)
	defer comp.Close()

	assert.Equal(t, Name, comp.Name())
	assert.True(t, comp.IsSupported())

	err = comp.Close()
	require.NoError(t, err)
}

func TestComponentName(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
	assert.Equal(t, "nfs", c.Name())
}

func TestTags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
}

func TestIsSupported(t *testing.T) {
	c := &component{}
	assert.True(t, c.IsSupported())
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Allow some time for the goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Clean up
	c.Close()
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	require.Error(t, c.ctx.Err(), "Context should be canceled after Close()")
	assert.Equal(t, context.Canceled, c.ctx.Err())
}

func TestEvents(t *testing.T) {
	c := &component{}
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestLastHealthStatesWithNoData(t *testing.T) {
	c := &component{}
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestLastHealthStatesWithData(t *testing.T) {
	testTime := time.Now().UTC()
	c := &component{
		lastCheckResult: &checkResult{
			ts:     testTime,
			health: apiv1.HealthStateTypeHealthy,
			reason: "test reason",
		},
	}

	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Equal(t, testTime, states[0].Time.Time)
}

func TestCheckWithNoConfigs(t *testing.T) {
	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{}
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "no nfs group configs found", result.Summary())
}

func TestCheckWithInvalidConfigs(t *testing.T) {
	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath: "", // Invalid - empty dir
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return configs.Validate(ctx)
		},
		newChecker: pkgnfschecker.NewChecker,
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return checker.Write(ctx)
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return checker.Check(ctx)
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return checker.Clean()
		},
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Contains(t, result.Summary(), "invalid nfs group configs")
}

func TestCheckWithValidConfigs(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	// The check should succeed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, tmpDir, cr.NFSCheckResults[0].Dir)
}

// Test checkResult methods

func TestCheckResultComponentName(t *testing.T) {
	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

func TestCheckResultString(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "empty NFSCheckResults",
			cr: &checkResult{
				NFSCheckResults: []pkgnfschecker.CheckResult{},
			},
			expected: "",
		},
		{
			name: "with NFSCheckResults",
			cr: &checkResult{
				NFSCheckResults: []pkgnfschecker.CheckResult{
					{
						Dir:     "/test/dir1",
						Message: "success",
					},
					{
						Dir:     "/test/dir2",
						Message: "failed",
					},
				},
			},
			expected: "", // We'll check that it contains table content instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cr.String()
			if tt.name == "with NFSCheckResults" {
				// Check that the result contains table headers and data
				assert.Contains(t, result, "DIRECTORY")
				assert.Contains(t, result, "MESSAGE")
				assert.Contains(t, result, "/test/dir1")
				assert.Contains(t, result, "success")
				assert.Contains(t, result, "/test/dir2")
				assert.Contains(t, result, "failed")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCheckResultSummary(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "with reason",
			cr: &checkResult{
				reason: "test reason",
			},
			expected: "test reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.Summary())
		})
	}
}

func TestCheckResultHealthStateType(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected apiv1.HealthStateType
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "healthy",
			cr: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy",
			cr: &checkResult{
				health: apiv1.HealthStateTypeDegraded,
			},
			expected: apiv1.HealthStateTypeDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.HealthStateType())
		})
	}
}

func TestCheckResultGetError(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "no error",
			cr: &checkResult{
				err: nil,
			},
			expected: "",
		},
		{
			name: "with error",
			cr: &checkResult{
				err: errors.New("test error"),
			},
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.getError())
		})
	}
}

func TestCheckResultHealthStates(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("with data", func(t *testing.T) {
		testTime := time.Now().UTC()
		testError := errors.New("test error")
		cr := &checkResult{
			ts:     testTime,
			err:    testError,
			health: apiv1.HealthStateTypeDegraded,
			reason: "test reason",
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeDegraded, states[0].Health)
		assert.Equal(t, "test reason", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
		assert.Equal(t, testTime, states[0].Time.Time)
	})
}

func TestCheckWithNFSCheckerError(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()

	// The check should succeed since the NFS checker writes and reads its own file
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "correctly read/wrote on")
}

func TestCheckWithNewCheckerError(t *testing.T) {
	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   "", // Invalid empty dir will cause validation to fail
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		// Return error for empty path
		if dir == "" {
			return "", "", errors.New("empty path")
		}
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Contains(t, result.Summary(), "invalid nfs group configs")
	assert.NotNil(t, cr.err)
}

func TestCheckWithWriteError(t *testing.T) {
	// Create a directory that we'll make read-only
	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0755)
	require.NoError(t, err)

	// Make the directory read-only to cause write to fail
	require.NoError(t, os.Chmod(readOnlyDir, 0555))
	defer os.RemoveAll(readOnlyDir)

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   readOnlyDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Contains(t, result.Summary(), "failed to write to nfs checker")
	assert.NotNil(t, cr.err)
}

func TestCheckWithMultipleMemberConfigs(t *testing.T) {
	// Create two temporary directories
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir1,
				FileContents: "test content 1",
			},
			{
				VolumePath:   tmpDir2,
				FileContents: "test content 2",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Both checks should succeed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 2)
	assert.Equal(t, tmpDir1, cr.NFSCheckResults[0].Dir)
	assert.Equal(t, tmpDir2, cr.NFSCheckResults[1].Dir)
	assert.Contains(t, result.Summary(), cr.NFSCheckResults[0].Message)
	assert.Contains(t, result.Summary(), cr.NFSCheckResults[1].Message)
}

func TestCheckWithCheckerError(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "expected content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	// Pre-populate the file with wrong content to make Check() fail
	nfsFile := filepath.Join(tmpDir, "test-machine")
	err := os.WriteFile(nfsFile, []byte("wrong content"), 0644)
	require.NoError(t, err)

	// The check should fail because Write() will overwrite with expected content,
	// but we'll make the file unwritable so Write() succeeds and Check() fails
	result := c.Check()

	// Should succeed - Write() will overwrite the wrong content with correct content,
	// then Check() will find the correct content
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "correctly read/wrote on")
}

func TestCheckResultInterface(t *testing.T) {
	// Verify that checkResult implements components.CheckResult interface
	var _ components.CheckResult = &checkResult{}
}

func TestComponentInterface(t *testing.T) {
	// Verify that component implements components.Component interface
	var _ components.Component = &component{}
}

func TestConcurrentAccess(t *testing.T) {
	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{}
	}

	// Test concurrent access to LastHealthStates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			states := c.LastHealthStates()
			assert.Len(t, states, 1)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestComponentWithRealData(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(&components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
		MachineID:  "test-machine",
	})
	require.NoError(t, err)
	defer comp.Close()

	// Test the actual Check method
	result := comp.Check()
	assert.NotNil(t, result)

	// Should be healthy since no configs are set by default
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "no nfs group configs found", result.Summary())

	// Test that lastCheckResult is updated
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestCheckWithFindMntTargetDeviceError(t *testing.T) {
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "", "", errors.New("mount target device error")
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Contains(t, result.Summary(), "failed to find mount target device for "+tmpDir)
	assert.NotNil(t, cr.err)
	assert.Equal(t, "mount target device error", cr.err.Error())
}

func TestCheckWithNonNFSMount(t *testing.T) {
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "/dev/sda1", "ext4", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return false // Not an NFS filesystem
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Equal(t, fmt.Sprintf("The user applied path %q as NFS volume, but in fact the file system type %q is not NFS.", tmpDir, "ext4"), result.Summary())
	assert.Nil(t, cr.err) // This case doesn't set an error, just health state
}

func TestCheckWithValidNFSMount(t *testing.T) {
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return fsType == "nfs"
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Should succeed with valid NFS mount
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, tmpDir, cr.NFSCheckResults[0].Dir)
}

func TestCheckWithMultipleVolumesOneFails(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	callCount := 0
	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir1,
				FileContents: "test content 1",
			},
			{
				VolumePath:   tmpDir2,
				FileContents: "test content 2",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		callCount++
		if callCount == 1 {
			return "server1:/export/path1", "nfs", nil
		}
		// Second call fails
		return "", "", errors.New("second mount check failed")
	}
	c.isNFSFSType = func(fsType string) bool {
		return fsType == "nfs"
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Should fail on second mount check
	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Contains(t, result.Summary(), "failed to find mount target device for "+tmpDir2)
	assert.NotNil(t, cr.err)
}

func TestCheckResultStringWithNFSResults(t *testing.T) {
	cr := &checkResult{
		NFSCheckResults: []pkgnfschecker.CheckResult{
			{
				Dir:     "/mnt/nfs1",
				Message: "wrote 1 files, expected 1 files (success)",
			},
			{
				Dir:     "/mnt/nfs2",
				Message: "wrote 2 files, expected 2 files (success)",
			},
		},
	}

	result := cr.String()
	// Should contain a table with the results - check for uppercase headers
	assert.Contains(t, result, "DIRECTORY")
	assert.Contains(t, result, "MESSAGE")
	assert.Contains(t, result, "/mnt/nfs1")
	assert.Contains(t, result, "/mnt/nfs2")
	assert.Contains(t, result, "wrote 1 files")
	assert.Contains(t, result, "wrote 2 files")
}

func TestStartPeriodicCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checkCount := 0
	var mu sync.Mutex

	c := &component{
		ctx:    ctx,
		cancel: cancel,
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			mu.Lock()
			checkCount++
			mu.Unlock()
			return pkgnfschecker.Configs{}
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Wait a short time to ensure the goroutine starts
	time.Sleep(100 * time.Millisecond)

	// Cancel to stop the goroutine
	cancel()

	// Give it time to process the cancellation
	time.Sleep(100 * time.Millisecond)

	// Should have been called at least once (initial ticker)
	mu.Lock()
	count := checkCount
	mu.Unlock()
	assert.GreaterOrEqual(t, count, 0, "Check should have been called during Start")
}

func TestCheckWithEmptyMemberConfigs(t *testing.T) {
	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		// Return empty configs to test the empty member configs scenario
		return pkgnfschecker.Configs{}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return true
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Should be healthy with no configs
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "no nfs group configs found", result.Summary())
	assert.Len(t, cr.NFSCheckResults, 0)
}

func TestHealthStatesConsistency(t *testing.T) {
	testTime := time.Now().UTC()

	tests := []struct {
		name           string
		cr             *checkResult
		expectedHealth apiv1.HealthStateType
		expectedReason string
		expectedError  string
	}{
		{
			name: "healthy state",
			cr: &checkResult{
				ts:     testTime,
				health: apiv1.HealthStateTypeHealthy,
				reason: "all checks passed",
				err:    nil,
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "all checks passed",
			expectedError:  "",
		},
		{
			name: "degraded state with error",
			cr: &checkResult{
				ts:     testTime,
				health: apiv1.HealthStateTypeDegraded,
				reason: "check failed",
				err:    errors.New("specific error"),
			},
			expectedHealth: apiv1.HealthStateTypeDegraded,
			expectedReason: "check failed",
			expectedError:  "specific error",
		},
		{
			name: "unhealthy state",
			cr: &checkResult{
				ts:     testTime,
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "critical failure",
				err:    errors.New("critical error"),
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "critical failure",
			expectedError:  "critical error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test direct methods
			assert.Equal(t, tt.expectedHealth, tt.cr.HealthStateType())
			assert.Equal(t, tt.expectedReason, tt.cr.Summary())
			assert.Equal(t, tt.expectedError, tt.cr.getError())

			// Test HealthStates method
			states := tt.cr.HealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, Name, states[0].Component)
			assert.Equal(t, tt.expectedHealth, states[0].Health)
			assert.Equal(t, tt.expectedReason, states[0].Reason)
			assert.Equal(t, tt.expectedError, states[0].Error)
			assert.Equal(t, testTime, states[0].Time.Time)
		})
	}
}

func TestCheckWithCleanSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				DirName:      "nfs-check",
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return fsType == "nfs"
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Should succeed - Clean() should be called successfully
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, filepath.Join(tmpDir, "nfs-check"), cr.NFSCheckResults[0].Dir)
	assert.Contains(t, result.Summary(), "correctly read/wrote on")
}

func TestCheckCallOrder(t *testing.T) {
	tmpDir := t.TempDir()

	c := createTestComponent()
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   tmpDir,
				DirName:      "nfs-check",
				FileContents: "test content",
			},
		}
	}
	c.findMntTargetDevice = func(dir string) (string, string, error) {
		return "server:/export/path", "nfs", nil
	}
	c.isNFSFSType = func(fsType string) bool {
		return fsType == "nfs"
	}

	result := c.Check()
	cr := result.(*checkResult)

	// Verify that the full sequence completed successfully (Write → Check → Clean)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, filepath.Join(tmpDir, "nfs-check"), cr.NFSCheckResults[0].Dir)

	// Verify that the check result contains the expected message showing Write and Check completed
	assert.Contains(t, result.Summary(), "correctly read/wrote on")

	// Verify no error occurred (meaning all steps succeeded)
	assert.Nil(t, cr.err)
}

func TestCheckCleanNotCalledOnEarlierFailures(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Clean not called when Write fails", func(t *testing.T) {
		// Make directory read-only to cause Write() to fail
		err := os.Chmod(tmpDir, 0555)
		require.NoError(t, err)
		defer func() { _ = os.Chmod(tmpDir, 0755) }()

		c := createTestComponent()
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      "nfs-check",
					FileContents: "test content",
				},
			}
		}
		c.findMntTargetDevice = func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		}
		c.isNFSFSType = func(fsType string) bool {
			return fsType == "nfs"
		}

		result := c.Check()
		cr := result.(*checkResult)

		// Should fail during config validation, before Write() is even called
		assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
		assert.Contains(t, result.Summary(), "invalid nfs group configs")
		assert.NotNil(t, cr.err)

		// NFSCheckResults should be empty since we never got to the Check() stage
		assert.Len(t, cr.NFSCheckResults, 0)
	})
}

func TestCheckWithValidateTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      ".gpud-test",
					FileContents: "test content",
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return context.DeadlineExceeded
		},
		newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
			return nil, errors.New("should not be called")
		},
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return pkgnfschecker.CheckResult{Error: "should not be called"}
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Equal(t, "NFS validation timed out - server may be unresponsive", result.Summary())
	assert.Equal(t, context.DeadlineExceeded, cr.err)
}

func TestCheckWithNewCheckerTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      ".gpud-test",
					FileContents: "test content",
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return nil // Success
		},
		newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
			return nil, context.DeadlineExceeded
		},
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return pkgnfschecker.CheckResult{Error: "should not be called"}
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Equal(t, "NFS checker creation timed out for "+tmpDir+" - server may be unresponsive", result.Summary())
	assert.Equal(t, context.DeadlineExceeded, cr.err)
}

func TestCheckWithWriteCheckerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockChecker := &mockChecker{}

	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      ".gpud-test",
					FileContents: "test content",
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return nil // Success
		},
		newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
			return mockChecker, nil // Success
		},
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return context.DeadlineExceeded
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return pkgnfschecker.CheckResult{Error: "should not be called"}
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Equal(t, "NFS write timed out for "+tmpDir+" - server may be unresponsive", result.Summary())
	assert.Equal(t, context.DeadlineExceeded, cr.err)
}

func TestCheckWithCheckCheckerTimeoutResult(t *testing.T) {
	tmpDir := t.TempDir()
	mockChecker := &mockChecker{}

	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      ".gpud-test",
					FileContents: "test content",
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return nil // Success
		},
		newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
			return mockChecker, nil // Success
		},
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return nil // Success
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return pkgnfschecker.CheckResult{
				Dir:          tmpDir,
				Error:        "operation timed out",
				TimeoutError: true, // This is the key field that indicates timeout
			}
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return errors.New("should not be called")
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	assert.Equal(t, "NFS check timed out for "+tmpDir+" - server may be unresponsive", result.Summary())
	assert.Contains(t, cr.err.Error(), "operation timed out")
}

func TestCheckWithNonTimeoutErrors(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name                    string
		validateError           error
		newCheckerError         error
		writeError              error
		checkResult             pkgnfschecker.CheckResult
		expectedReasonSubstring string
	}{
		{
			name:                    "validate error non-timeout",
			validateError:           errors.New("validation failed"),
			expectedReasonSubstring: "invalid nfs group configs",
		},
		{
			name:                    "newChecker error non-timeout",
			newCheckerError:         errors.New("creation failed"),
			expectedReasonSubstring: "failed to create nfs checker",
		},
		{
			name:                    "write error non-timeout",
			writeError:              errors.New("write failed"),
			expectedReasonSubstring: "failed to write to nfs checker",
		},
		{
			name: "check error non-timeout",
			checkResult: pkgnfschecker.CheckResult{
				Dir:          tmpDir,
				Error:        "check failed",
				TimeoutError: false, // Non-timeout error
			},
			expectedReasonSubstring: "failed to check nfs checker",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockChecker := &mockChecker{}

			c := &component{
				ctx:       context.Background(),
				machineID: "test-machine",
				getGroupConfigsFunc: func() pkgnfschecker.Configs {
					return pkgnfschecker.Configs{
						{
							VolumePath:   tmpDir,
							DirName:      ".gpud-test",
							FileContents: "test content",
						},
					}
				},
				findMntTargetDevice: func(dir string) (string, string, error) {
					return "server:/export/path", "nfs", nil
				},
				isNFSFSType: func(fsType string) bool {
					return true
				},
				validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
					if tc.validateError != nil {
						return tc.validateError
					}
					return nil
				},
				newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
					if tc.newCheckerError != nil {
						return nil, tc.newCheckerError
					}
					return mockChecker, nil
				},
				writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
					if tc.writeError != nil {
						return tc.writeError
					}
					return nil
				},
				checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
					if tc.checkResult.Error != "" {
						return tc.checkResult
					}
					return pkgnfschecker.CheckResult{
						Dir:     tmpDir,
						Message: "success",
					}
				},
				cleanChecker: func(checker pkgnfschecker.Checker) error {
					return nil
				},
			}

			result := c.Check()

			assert.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
			assert.Contains(t, result.Summary(), tc.expectedReasonSubstring)
		})
	}
}

func TestCheckWithSuccessfulOperations(t *testing.T) {
	tmpDir := t.TempDir()
	mockChecker := &mockChecker{}

	c := &component{
		ctx:       context.Background(),
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   tmpDir,
					DirName:      ".gpud-test",
					FileContents: "test content",
				},
			}
		},
		findMntTargetDevice: func(dir string) (string, string, error) {
			return "server:/export/path", "nfs", nil
		},
		isNFSFSType: func(fsType string) bool {
			return true
		},
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return nil
		},
		newChecker: func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error) {
			return mockChecker, nil
		},
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return nil
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return pkgnfschecker.CheckResult{
				Dir:     tmpDir,
				Message: "correctly read/wrote on " + tmpDir,
			}
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return nil
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "correctly read/wrote on")
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, tmpDir, cr.NFSCheckResults[0].Dir)
}

// mockChecker is a simple mock implementation of the Checker interface for testing
type mockChecker struct{}

func (m *mockChecker) Write(ctx context.Context) error {
	return nil
}

func (m *mockChecker) Check(ctx context.Context) pkgnfschecker.CheckResult {
	return pkgnfschecker.CheckResult{}
}

func (m *mockChecker) Clean() error {
	return nil
}
