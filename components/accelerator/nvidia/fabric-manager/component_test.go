package fabricmanager

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)

	bucket, err := store.Bucket(Name)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := newWatcher([][]string{
		{"tail", "testdata/fabricmanager.log"},
		{"sleep 1"},
	})
	require.NoError(t, err)
	llp := newLogLineProcessor(ctx, w, Match, bucket)

	comp := &component{
		ctx:    ctx,
		cancel: cancel,

		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },

		eventBucket:      bucket,
		logLineProcessor: llp,
	}
	defer comp.Close()

	_ = comp.Check()

	time.Sleep(5 * time.Second)

	events, err := comp.Events(ctx, time.Time{})
	require.NoError(t, err)
	assert.Len(t, events, 1)

	expectedEvent := apiv1.Event{
		Time:    metav1.Time{Time: time.Date(2025, 2, 27, 15, 10, 2, 0, time.UTC)},
		Name:    "fabricmanager_nvswitch_non_fatal_error",
		Type:    "Warning",
		Message: "NVSwitch non-fatal error detected",
	}

	assert.Equal(t, expectedEvent.Name, events[0].Name)
	assert.Equal(t, expectedEvent.Type, events[0].Type)
	assert.Equal(t, expectedEvent.Message, events[0].Message)

	comp.checkFMExistsFunc = func() bool { return false }
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
}

func TestCheckFMExists(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		assert.False(t, checkFMExists())
	})

	t.Run("found", func(t *testing.T) {
		tempDir := t.TempDir()
		binPath := filepath.Join(tempDir, "nv-fabricmanager")
		require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
		t.Setenv("PATH", tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		assert.True(t, checkFMExists())
	})
}

func TestCheckFMActive(t *testing.T) {
	t.Run("port inactive", func(t *testing.T) {
		assert.False(t, checkFMActive())
	})

	t.Run("port active", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:6666")
		require.NoError(t, err)
		defer listener.Close()

		assert.True(t, checkFMActive())
	})
}

// mockWatcher implements the watcher interface for testing
type mockWatcher struct {
	ch chan logLine
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		ch: make(chan logLine),
	}
}

func (w *mockWatcher) watch() <-chan logLine {
	return w.ch
}

func (w *mockWatcher) close() {
	close(w.ch)
}

// mockMatchFunc implements the matchFunc interface for testing
func mockMatchFunc(line string) (eventName string, message string) {
	if line == "test-error-line" {
		return "test-error", "This is a test error"
	}
	return "", ""
}

func TestEventsWithNoProcessor(t *testing.T) {
	t.Parallel()

	// Create a component with no logLineProcessor
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkFMExistsFunc: func() bool { return false },
		checkFMActiveFunc: func() bool { return false },
	}

	// Call Events
	events, err := comp.Events(context.Background(), time.Now().Add(-1*time.Hour))

	// Verify results
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestEventsWithProcessor(t *testing.T) {
	t.Parallel()

	// Setup SQLite database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock watcher
	mockW := newMockWatcher()

	// Create events store
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)

	// Create a processor
	llp := newLogLineProcessor(ctx, mockW, mockMatchFunc, bucket)

	// Create component with processor
	comp := &component{
		ctx:    ctx,
		cancel: cancel,

		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },

		eventBucket:      bucket,
		logLineProcessor: llp,
	}

	// Insert a test event directly into the store
	testEvent := eventstore.Event{
		Time:    time.Now().Add(-30 * time.Minute),
		Name:    "test-error",
		Message: "This is a test error",
		Type:    "Warning",
	}
	err = bucket.Insert(ctx, testEvent)
	require.NoError(t, err)

	// Call Events
	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))

	// Verify results
	assert.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-error", events[0].Name)
	assert.Equal(t, "This is a test error", events[0].Message)
}

func TestStatesWhenFabricManagerDoesNotExist(t *testing.T) {
	t.Parallel()

	// Create a component where fabric manager doesn't exist
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return false },
		checkFMActiveFunc:       func() bool { return false },
	}

	_ = comp.Check()

	// Call States
	states := comp.LastHealthStates()

	// Verify results
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "nv-fabricmanager executable not found", states[0].Reason)
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	comp := &component{}
	assert.Equal(t, Name, comp.Name())
}

func TestTags(t *testing.T) {
	t.Parallel()

	comp := &component{}

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := comp.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

func TestIsSupported(t *testing.T) {
	t.Parallel()

	// Test when nvmlInstance is nil
	comp := &component{
		nvmlInstance: nil,
	}
	assert.False(t, comp.IsSupported())

	// Test when NVMLExists returns false
	comp = &component{
		nvmlInstance: &mockNVMLInstance{exists: false, productName: "", deviceCount: 0},
	}
	assert.False(t, comp.IsSupported())

	// Test when ProductName returns empty string
	comp = &component{
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "", deviceCount: 0},
	}
	assert.False(t, comp.IsSupported())

	// Test when all conditions are met
	comp = &component{
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100", deviceCount: 1},
	}
	assert.True(t, comp.IsSupported())
}

func TestComponentStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	comp := &component{
		ctx:               ctx,
		cancel:            cancel,
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },
	}
	defer comp.Close()

	err := comp.Start()
	assert.NoError(t, err)

	// Allow time for the goroutine to do first check
	time.Sleep(100 * time.Millisecond)

	// Verify lastCheckResult was updated
	comp.lastMu.RLock()
	assert.NotNil(t, comp.lastCheckResult)
	comp.lastMu.RUnlock()
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	// Setup mock components
	ctx, cancel := context.WithCancel(context.Background())
	mockW := newMockWatcher()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)

	llp := newLogLineProcessor(ctx, mockW, mockMatchFunc, bucket)

	comp := &component{
		ctx:              ctx,
		cancel:           cancel,
		logLineProcessor: llp,
		eventBucket:      bucket,
	}

	// Test Close
	err = comp.Close()
	assert.NoError(t, err)
}

func TestStatesWhenFabricManagerExistsButNotActive(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:                     context.Background(),
		cancel:                  func() {},
		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return false },
	}

	_ = comp.Check()

	states := comp.LastHealthStates()

	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "nv-fabricmanager found but fabric manager service is not active", states[0].Reason)
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil Data
	var cr *checkResult
	assert.Equal(t, "", cr.getError())

	// Test nil error
	cr = &checkResult{}
	assert.Equal(t, "", cr.getError())

	// Test with error
	testErr := assert.AnError
	cr = &checkResult{err: testErr}
	assert.Equal(t, testErr.Error(), cr.getError())
}

func TestDataGetLastHealthStates(t *testing.T) {
	t.Parallel()

	// Test nil Data
	var cr *checkResult
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test unhealthy state
	cr = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test unhealthy reason",
		err:    assert.AnError,
	}
	states = cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test unhealthy reason", states[0].Reason)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Test creating component with nil eventstore
	instance := &components.GPUdInstance{
		RootCtx: context.Background(),
	}
	comp, err := New(instance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestDataString(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.String())

	// Test with active fabric manager
	cr = &checkResult{
		FabricManagerActive: true,
	}
	assert.Equal(t, "fabric manager is active", cr.String())

	// Test with inactive fabric manager
	cr = &checkResult{
		FabricManagerActive: false,
	}
	assert.Equal(t, "fabric manager is not active", cr.String())

	// Test rendering fabric state table output
	cr = &checkResult{
		FabricStateSupported: true,
		FabricStates: []fabricStateEntry{
			{
				GPUUUID:  "GPU-0",
				CliqueID: 4026,
				State:    "Completed",
				Status:   "Success",
				Health: fabricHealthSnapshot{
					Bandwidth: "Full",
				},
			},
		},
	}
	output := cr.String()
	assert.True(t, strings.Contains(output, "GPU-0"), output)
	assert.True(t, strings.Contains(output, "4026"), output)
}

func TestDataSummary(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.Summary())

	// Test with reason
	cr = &checkResult{
		reason: "test reason",
	}
	assert.Equal(t, "test reason", cr.Summary())
}

func TestCheckResultComponentName(t *testing.T) {
	t.Parallel()

	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

func TestDataHealthState(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())

	// Test with health state
	cr = &checkResult{
		health: apiv1.HealthStateTypeHealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
}

func TestStatesWhenFabricManagerExistsAndActive(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },
	}

	result := comp.Check()
	assert.NotNil(t, result)

	// Type assertion to access Data methods
	data, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, data.FabricManagerActive)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "fabric manager found and active", data.reason)

	states := comp.LastHealthStates()
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
}

// This test mocks checkFMExists and checkFMActive to test all branches in Check method
func TestCheckAllBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		checkNVSwitchExists bool
		fmExists            bool
		fmActive            bool
		expectedData        *checkResult
		expectedState       apiv1.HealthStateType
		expectedReason      string
		expectedFMActive    bool
	}{
		{
			name:                "NVSwitch doesn't exist",
			checkNVSwitchExists: false,
			fmExists:            false,
			fmActive:            false,
			expectedState:       apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "NVSwitch exists but FM doesn't exist",
			checkNVSwitchExists: true,
			fmExists:            false,
			fmActive:            false,
			expectedState:       apiv1.HealthStateTypeHealthy,
			expectedReason:      "nv-fabricmanager executable not found",
			expectedFMActive:    false,
		},
		{
			name:                "NVSwitch exists, FM exists but not active",
			checkNVSwitchExists: true,
			fmExists:            true,
			fmActive:            false,
			expectedState:       apiv1.HealthStateTypeUnhealthy,
			expectedReason:      "nv-fabricmanager found but fabric manager service is not active",
			expectedFMActive:    false,
		},
		{
			name:                "NVSwitch exists, FM exists and active",
			checkNVSwitchExists: true,
			fmExists:            true,
			fmActive:            true,
			expectedState:       apiv1.HealthStateTypeHealthy,
			expectedReason:      "fabric manager found and active",
			expectedFMActive:    true,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			comp := &component{
				ctx:                     context.Background(),
				cancel:                  func() {},
				nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
				checkNVSwitchExistsFunc: func() bool { return tc.checkNVSwitchExists },
				checkFMExistsFunc:       func() bool { return tc.fmExists },
				checkFMActiveFunc:       func() bool { return tc.fmActive },
			}

			result := comp.Check()
			data, ok := result.(*checkResult)
			assert.True(t, ok)

			assert.Equal(t, tc.expectedFMActive, data.FabricManagerActive)
			assert.Equal(t, tc.expectedState, data.health)
			assert.Equal(t, tc.expectedReason, data.reason)
		})
	}
}

// mockNVMLInstance implements nvidianvml.Instance for testing
type mockNVMLInstance struct {
	exists              bool
	supportsFM          bool
	supportsFabricState bool
	productName         string
	architecture        string
	deviceCount         int // Add device count field
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	// Return a map with the specified number of mock devices
	if m.deviceCount <= 0 {
		return make(map[string]device.Device)
	}

	devices := make(map[string]device.Device)
	for i := 0; i < m.deviceCount; i++ {
		devices[fmt.Sprintf("device-%d", i)] = nil // Using nil for simplicity since we only need the count
	}
	return devices
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) Architecture() string {
	return m.architecture
}

func (m *mockNVMLInstance) Brand() string {
	return ""
}

func (m *mockNVMLInstance) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return m.supportsFM
}

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return m.supportsFabricState
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func TestComponentCheck_NVMLInstance(t *testing.T) {
	tests := []struct {
		name                    string
		nvmlInstance            nvidianvml.Instance
		expectedHealth          apiv1.HealthStateType
		expectedReason          string
		checkNVSwitchExistsFunc func() bool
		checkFMExistsFunc       func() bool
		checkFMActiveFunc       func() bool
	}{
		{
			name:                    "nil nvml instance",
			nvmlInstance:            nil,
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "NVIDIA NVML instance is nil",
			checkNVSwitchExistsFunc: func() bool { return false },
			checkFMExistsFunc:       func() bool { return false },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "nvml does not exist",
			nvmlInstance:            &mockNVMLInstance{exists: false, supportsFM: true, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "NVIDIA NVML library is not loaded",
			checkNVSwitchExistsFunc: func() bool { return false },
			checkFMExistsFunc:       func() bool { return false },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "fabric manager not supported",
			nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: false, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "Test GPU does not support fabric manager",
			checkNVSwitchExistsFunc: func() bool { return false },
			checkFMExistsFunc:       func() bool { return false },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "nvml exists but NVSwitch not found",
			nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "NVSwitch not detected, skipping fabric manager check",
			checkNVSwitchExistsFunc: func() bool { return false },
			checkFMExistsFunc:       func() bool { return false },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "nvml exists with NVSwitch but FM executable not found",
			nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "nv-fabricmanager executable not found",
			checkNVSwitchExistsFunc: func() bool { return true },
			checkFMExistsFunc:       func() bool { return false },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "nvml exists, NVSwitch found, FM executable found but not active",
			nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeUnhealthy,
			expectedReason:          "nv-fabricmanager found but fabric manager service is not active",
			checkNVSwitchExistsFunc: func() bool { return true },
			checkFMExistsFunc:       func() bool { return true },
			checkFMActiveFunc:       func() bool { return false },
		},
		{
			name:                    "nvml exists, NVSwitch found, FM executable found and active",
			nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "fabric manager found and active",
			checkNVSwitchExistsFunc: func() bool { return true },
			checkFMExistsFunc:       func() bool { return true },
			checkFMActiveFunc:       func() bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:                     ctx,
				cancel:                  cancel,
				nvmlInstance:            tt.nvmlInstance,
				checkNVSwitchExistsFunc: tt.checkNVSwitchExistsFunc,
				checkFMExistsFunc:       tt.checkFMExistsFunc,
				checkFMActiveFunc:       tt.checkFMActiveFunc,
			}

			result := c.Check()
			checkResult, ok := result.(*checkResult)
			assert.True(t, ok, "Expected result to be of type *checkResult")

			assert.Equal(t, tt.expectedHealth, checkResult.health)
			assert.Equal(t, tt.expectedReason, checkResult.reason)

			// Additional checks for specific states
			if tt.nvmlInstance != nil && tt.nvmlInstance.NVMLExists() && tt.nvmlInstance.FabricManagerSupported() {
				if tt.checkNVSwitchExistsFunc() {
					if tt.checkFMExistsFunc() {
						if tt.checkFMActiveFunc() {
							assert.True(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be true")
						} else {
							assert.False(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be false")
						}
					} else {
						assert.False(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be false")
					}
				} else {
					assert.False(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be false when NVSwitch not found")
				}
			}
		})
	}
}

func TestCheck_FabricManagerNotSupported(t *testing.T) {
	t.Parallel()

	// Create a mock NVML instance that doesn't support fabric manager
	mockInstance := &mockNVMLInstance{
		exists:      true,
		supportsFM:  false,
		productName: "Test GPU",
		deviceCount: 2,
	}

	// Create the component with our mock instance
	comp := &component{
		ctx:               context.Background(),
		cancel:            func() {},
		nvmlInstance:      mockInstance,
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },
	}

	// Call Check method
	result := comp.Check()

	// Verify the result
	checkResult, ok := result.(*checkResult)
	assert.True(t, ok, "Expected result to be of type *checkResult")

	// Verify all expected values
	assert.False(t, checkResult.FabricManagerActive)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "Test GPU does not support fabric manager", checkResult.reason)

	// Also verify the health states output
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "Test GPU does not support fabric manager", states[0].Reason)
}

func TestCheckWithEmptyProductName(t *testing.T) {
	// Create mock NVML instance with empty product name
	mockNVML := &mockNVMLInstance{
		exists:      true,
		supportsFM:  false,
		productName: "", // empty product name
		deviceCount: 2,
	}

	// Create component with mock
	c := &component{
		ctx:          context.Background(),
		nvmlInstance: mockNVML,
	}

	// Call Check
	result := c.Check()

	// Assert on result
	checkResult, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", checkResult.reason)
}

func TestCheckDeviceCountLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		deviceCount         int
		checkNVSwitchExists bool
		expectedHealth      apiv1.HealthStateType
		expectedReason      string
		expectedFMActive    bool
	}{
		{
			name:                "no devices detected - NVSwitch not found",
			deviceCount:         0,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "one device detected - NVSwitch not found",
			deviceCount:         1,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "two devices detected - NVSwitch found - FM not active",
			deviceCount:         2,
			checkNVSwitchExists: true,
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReason:      "nv-fabricmanager found but fabric manager service is not active",
			expectedFMActive:    false,
		},
		{
			name:                "two devices detected - NVSwitch not found",
			deviceCount:         2,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "multiple devices detected - NVSwitch found - FM not active",
			deviceCount:         4,
			checkNVSwitchExists: true,
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReason:      "nv-fabricmanager found but fabric manager service is not active",
			expectedFMActive:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create mock NVML instance with specified device count
			mockNVML := &mockNVMLInstance{
				exists:      true,
				supportsFM:  true,
				productName: "Test GPU",
				deviceCount: tc.deviceCount,
			}

			// Create component where fabric manager exists but is not active
			comp := &component{
				ctx:                     context.Background(),
				cancel:                  func() {},
				nvmlInstance:            mockNVML,
				checkNVSwitchExistsFunc: func() bool { return tc.checkNVSwitchExists },
				checkFMExistsFunc:       func() bool { return true },  // FM exists
				checkFMActiveFunc:       func() bool { return false }, // FM not active
			}

			// Call Check method
			result := comp.Check()

			// Verify the result
			checkResult, ok := result.(*checkResult)
			assert.True(t, ok, "Expected result to be of type *checkResult")

			// Verify health and reason
			assert.Equal(t, tc.expectedHealth, checkResult.health, "Health state mismatch for %d devices", tc.deviceCount)
			assert.Equal(t, tc.expectedReason, checkResult.reason, "Reason mismatch for %d devices", tc.deviceCount)

			// Verify FabricManagerActive
			assert.Equal(t, tc.expectedFMActive, checkResult.FabricManagerActive, "FabricManagerActive mismatch for %d devices", tc.deviceCount)

			// Also verify through LastHealthStates()
			states := comp.LastHealthStates()
			assert.Len(t, states, 1)
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
			assert.Equal(t, tc.expectedReason, states[0].Reason)
		})
	}
}

func TestCheckDeviceCountWithActiveManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		deviceCount         int
		checkNVSwitchExists bool
		expectedHealth      apiv1.HealthStateType
		expectedReason      string
		expectedFMActive    bool
	}{
		{
			name:                "Single device with NVSwitch not found",
			deviceCount:         1,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "Single device with NVSwitch found and FM active",
			deviceCount:         1,
			checkNVSwitchExists: true,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "fabric manager found and active",
			expectedFMActive:    true,
		},
		{
			name:                "Multiple devices with NVSwitch found and FM active",
			deviceCount:         2,
			checkNVSwitchExists: true,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "fabric manager found and active",
			expectedFMActive:    true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockNVML := &mockNVMLInstance{
				exists:      true,
				supportsFM:  true,
				productName: "Test GPU",
				deviceCount: tc.deviceCount,
			}

			comp := &component{
				ctx:                     context.Background(),
				cancel:                  func() {},
				nvmlInstance:            mockNVML,
				checkNVSwitchExistsFunc: func() bool { return tc.checkNVSwitchExists },
				checkFMExistsFunc:       func() bool { return true }, // FM exists
				checkFMActiveFunc:       func() bool { return true }, // FM is active
			}

			result := comp.Check()
			checkResult, ok := result.(*checkResult)
			assert.True(t, ok)

			assert.Equal(t, tc.expectedHealth, checkResult.health)
			assert.Equal(t, tc.expectedReason, checkResult.reason)
			assert.Equal(t, tc.expectedFMActive, checkResult.FabricManagerActive)
		})
	}
}

func TestCheckNVSwitchNotDetected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		deviceCount         int
		checkNVSwitchExists bool
		expectedHealth      apiv1.HealthStateType
		expectedReason      string
		expectedFMActive    bool
	}{
		{
			name:                "NVSwitch not detected with single device",
			deviceCount:         1,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "NVSwitch not detected with multiple devices",
			deviceCount:         4,
			checkNVSwitchExists: false,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "NVSwitch not detected, skipping fabric manager check",
			expectedFMActive:    false,
		},
		{
			name:                "NVSwitch detected but FM not found",
			deviceCount:         4,
			checkNVSwitchExists: true,
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      "nv-fabricmanager executable not found",
			expectedFMActive:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create mock NVML instance
			mockNVML := &mockNVMLInstance{
				exists:      true,
				supportsFM:  true,
				productName: "Test GPU",
				deviceCount: tc.deviceCount,
			}

			// Create component
			comp := &component{
				ctx:                     context.Background(),
				cancel:                  func() {},
				nvmlInstance:            mockNVML,
				checkNVSwitchExistsFunc: func() bool { return tc.checkNVSwitchExists },
				checkFMExistsFunc:       func() bool { return false }, // FM doesn't exist
				checkFMActiveFunc:       func() bool { return false }, // FM not active
			}

			// Call Check method
			result := comp.Check()

			// Verify the result
			checkResult, ok := result.(*checkResult)
			assert.True(t, ok, "Expected result to be of type *checkResult")

			// Verify health and reason
			assert.Equal(t, tc.expectedHealth, checkResult.health)
			assert.Equal(t, tc.expectedReason, checkResult.reason)
			assert.Equal(t, tc.expectedFMActive, checkResult.FabricManagerActive)

			// Also verify through LastHealthStates()
			states := comp.LastHealthStates()
			assert.Len(t, states, 1)
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
			assert.Equal(t, tc.expectedReason, states[0].Reason)
		})
	}
}

func TestCheckNVSwitchFuncNil(t *testing.T) {
	t.Parallel()

	// Create component with nil checkNVSwitchExistsFunc
	comp := &component{
		ctx:                     context.Background(),
		cancel:                  func() {},
		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: nil, // nil function
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },
	}

	// Call Check method
	result := comp.Check()

	// Verify the result - when checkNVSwitchExistsFunc is nil, it should proceed to check FM
	checkResult, ok := result.(*checkResult)
	assert.True(t, ok, "Expected result to be of type *checkResult")

	// Should proceed to check FM and find it active
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "fabric manager found and active", checkResult.reason)
	assert.True(t, checkResult.FabricManagerActive)
}

// TestCheck_FabricStateUnhealthy_ComponentBecomesUnhealthy tests that when fabric state
// reports unhealthy (report.Healthy = false), the component health state becomes unhealthy
func TestCheck_FabricStateSkippedWhenSingleGPU(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sawFabricCall := false

	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA H100",
			deviceCount:         1,
		},
		collectFabricStateFunc: func() fabricStateReport {
			sawFabricCall = true
			return fabricStateReport{Healthy: true}
		},
		getCountLspci: func(context.Context) (int, error) {
			return 1, nil
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok, "Expected result to be of type *checkResult")

	assert.False(t, sawFabricCall, "fabric state collection must be skipped for single GPU")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "detected 1 NVIDIA GPU device(s); skipping fabric state check", cr.reason)
	assert.Equal(t, cr.reason, cr.FabricStateReason)
	assert.True(t, cr.FabricStateSupported)
}

func TestCheck_FabricStateUnhealthy_ComponentBecomesUnhealthy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		productName         string
		fabricStateReport   fabricStateReport
		expectedHealth      apiv1.HealthStateType
		expectedReasonMatch string // substring to match in reason
	}{
		{
			name:        "unhealthy with reason - bandwidth degraded",
			productName: "NVIDIA GB200",
			fabricStateReport: fabricStateReport{
				Healthy: false,
				Reason:  "GPU GPU-0: bandwidth degraded",
				Entries: []fabricStateEntry{{GPUUUID: "GPU-0"}},
			},
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReasonMatch: "NVIDIA GB200 with unhealthy fabric state: GPU GPU-0: bandwidth degraded",
		},
		{
			name:        "unhealthy with error only",
			productName: "NVIDIA GB200 NVL72",
			fabricStateReport: fabricStateReport{
				Healthy: false,
				Reason:  "",
				Err:     assert.AnError,
			},
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReasonMatch: "NVIDIA GB200 NVL72 with unhealthy fabric state: ",
		},
		{
			name:        "unhealthy with both reason and error",
			productName: "NVIDIA H100",
			fabricStateReport: fabricStateReport{
				Healthy: false,
				Reason:  "GPU GPU-1: route unhealthy",
				Err:     assert.AnError,
				Entries: []fabricStateEntry{{GPUUUID: "GPU-1"}},
			},
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReasonMatch: "NVIDIA H100 with unhealthy fabric state: GPU GPU-1: route unhealthy",
		},
		{
			name:        "unhealthy with multiple GPUs",
			productName: "NVIDIA GB200",
			fabricStateReport: fabricStateReport{
				Healthy: false,
				Reason:  "GPU GPU-0: bandwidth degraded; GPU GPU-1: route recovery in progress",
				Entries: []fabricStateEntry{
					{GPUUUID: "GPU-0"},
					{GPUUUID: "GPU-1"},
				},
			},
			expectedHealth:      apiv1.HealthStateTypeUnhealthy,
			expectedReasonMatch: "NVIDIA GB200 with unhealthy fabric state: GPU GPU-0: bandwidth degraded; GPU GPU-1: route recovery in progress",
		},
		{
			name:        "healthy state - should be healthy",
			productName: "NVIDIA GB200",
			fabricStateReport: fabricStateReport{
				Healthy: true,
				Reason:  "",
				Entries: []fabricStateEntry{{GPUUUID: "GPU-0", State: "Completed"}},
			},
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReasonMatch: "NVIDIA GB200 checked fabric state",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			comp := &component{
				ctx:    context.Background(),
				cancel: func() {},
				nvmlInstance: &mockNVMLInstance{
					exists:              true,
					supportsFM:          false,
					supportsFabricState: true,
					productName:         tc.productName,
					deviceCount:         2,
				},
				collectFabricStateFunc: func() fabricStateReport {
					return tc.fabricStateReport
				},
			}

			result := comp.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok, "Expected result to be of type *checkResult")

			// Verify health state matches expected
			assert.Equal(t, tc.expectedHealth, cr.health,
				"Health state should be %s when fabric state Healthy=%v",
				tc.expectedHealth, tc.fabricStateReport.Healthy)

			// Verify reason contains expected substring
			assert.Equal(t, tc.expectedReasonMatch, cr.reason,
				"Reason should match expected pattern")

			// Verify FabricStateSupported is true
			assert.True(t, cr.FabricStateSupported,
				"FabricStateSupported should be true")

			// Verify FabricStateReason is set correctly
			assert.Equal(t, tc.fabricStateReport.Reason, cr.FabricStateReason,
				"FabricStateReason should match report reason")

			// Verify via LastHealthStates as well
			states := comp.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health,
				"Health state via LastHealthStates should match")
			assert.Equal(t, tc.expectedReasonMatch, states[0].Reason,
				"Reason via LastHealthStates should match")
		})
	}
}

// TestCheck_FabricStateUnhealthyLogging verifies that when fabric state is unhealthy,
// appropriate warning is logged
func TestCheck_FabricStateUnhealthyLogging(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200",
			deviceCount:         1,
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Healthy: false,
				Reason:  "test unhealthy reason",
				Err:     assert.AnError,
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify the component is unhealthy
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "with unhealthy fabric state: test unhealthy reason")

	// The logging happens in the Check method - we can't directly verify logs in tests,
	// but we can verify the behavior is correct
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, "test unhealthy reason", cr.FabricStateReason)
}
