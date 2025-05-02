package fabricmanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
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

		loadNVML:          &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },

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
		DeprecatedExtraInfo: map[string]string{
			"log_line": "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
		},
	}

	assert.Equal(t, expectedEvent.Name, events[0].Name)
	assert.Equal(t, expectedEvent.Type, events[0].Type)
	assert.Equal(t, expectedEvent.Message, events[0].Message)
	assert.Equal(t, expectedEvent.DeprecatedExtraInfo["log_line"], events[0].DeprecatedExtraInfo["log_line"])

	comp.checkFMExistsFunc = func() bool { return false }
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
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
	testEvent := apiv1.Event{
		Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
		Name:    "test-error",
		Message: "This is a test error",
		Type:    "Warning",
		DeprecatedExtraInfo: map[string]string{
			"log_line": "test-error-line",
		},
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

		loadNVML:          &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
		checkFMExistsFunc: func() bool { return false },
		checkFMActiveFunc: func() bool { return false },
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
		ctx:               context.Background(),
		cancel:            func() {},
		loadNVML:          &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return false },
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

		loadNVML:          &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },
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
		name           string
		fmExists       bool
		fmActive       bool
		expectedData   *checkResult
		expectedState  apiv1.HealthStateType
		expectedReason string
	}{
		{
			name:           "FM doesn't exist",
			fmExists:       false,
			fmActive:       false,
			expectedState:  apiv1.HealthStateTypeHealthy,
			expectedReason: "nv-fabricmanager executable not found",
		},
		{
			name:           "FM exists but not active",
			fmExists:       true,
			fmActive:       false,
			expectedState:  apiv1.HealthStateTypeUnhealthy,
			expectedReason: "nv-fabricmanager found but fabric manager service is not active",
		},
		{
			name:           "FM exists and active",
			fmExists:       true,
			fmActive:       true,
			expectedState:  apiv1.HealthStateTypeHealthy,
			expectedReason: "fabric manager found and active",
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			comp := &component{
				ctx:               context.Background(),
				cancel:            func() {},
				loadNVML:          &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
				checkFMExistsFunc: func() bool { return tc.fmExists },
				checkFMActiveFunc: func() bool { return tc.fmActive },
			}

			result := comp.Check()
			data, ok := result.(*checkResult)
			assert.True(t, ok)

			if tc.fmExists && tc.fmActive {
				assert.True(t, data.FabricManagerActive)
			} else {
				assert.False(t, data.FabricManagerActive)
			}

			assert.Equal(t, tc.expectedState, data.health)
			assert.Equal(t, tc.expectedReason, data.reason)
		})
	}
}

// mockNVMLInstance implements nvidianvml.Instance for testing
type mockNVMLInstance struct {
	exists      bool
	supportsFM  bool
	productName string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) Architecture() string {
	return ""
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

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func TestComponentCheck_NVMLInstance(t *testing.T) {
	tests := []struct {
		name              string
		nvmlInstance      nvidianvml.Instance
		expectedHealth    apiv1.HealthStateType
		expectedReason    string
		checkFMExistsFunc func() bool
		checkFMActiveFunc func() bool
	}{
		{
			name:              "nil nvml instance",
			nvmlInstance:      nil,
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    "NVIDIA NVML instance is nil",
			checkFMExistsFunc: func() bool { return false },
			checkFMActiveFunc: func() bool { return false },
		},
		{
			name:              "nvml does not exist",
			nvmlInstance:      &mockNVMLInstance{exists: false, supportsFM: true, productName: "Test GPU"},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    "NVIDIA NVML library is not loaded",
			checkFMExistsFunc: func() bool { return false },
			checkFMActiveFunc: func() bool { return false },
		},
		{
			name:              "fabric manager not supported",
			nvmlInstance:      &mockNVMLInstance{exists: true, supportsFM: false, productName: "Test GPU"},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    "Test GPU does not support fabric manager",
			checkFMExistsFunc: func() bool { return false },
			checkFMActiveFunc: func() bool { return false },
		},
		{
			name:              "nvml exists but FM executable not found",
			nvmlInstance:      &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    "nv-fabricmanager executable not found",
			checkFMExistsFunc: func() bool { return false },
			checkFMActiveFunc: func() bool { return false },
		},
		{
			name:              "nvml exists, FM executable found but not active",
			nvmlInstance:      &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReason:    "nv-fabricmanager found but fabric manager service is not active",
			checkFMExistsFunc: func() bool { return true },
			checkFMActiveFunc: func() bool { return false },
		},
		{
			name:              "nvml exists, FM executable found and active",
			nvmlInstance:      &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU"},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    "fabric manager found and active",
			checkFMExistsFunc: func() bool { return true },
			checkFMActiveFunc: func() bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:               ctx,
				cancel:            cancel,
				loadNVML:          tt.nvmlInstance,
				checkFMExistsFunc: tt.checkFMExistsFunc,
				checkFMActiveFunc: tt.checkFMActiveFunc,
			}

			result := c.Check()
			checkResult, ok := result.(*checkResult)
			assert.True(t, ok, "Expected result to be of type *checkResult")

			assert.Equal(t, tt.expectedHealth, checkResult.health)
			assert.Equal(t, tt.expectedReason, checkResult.reason)

			// Additional checks for specific states
			if tt.nvmlInstance != nil && tt.nvmlInstance.NVMLExists() {
				if tt.checkFMExistsFunc() {
					if tt.checkFMActiveFunc() {
						assert.True(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be true")
					} else {
						assert.False(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be false")
					}
				} else {
					assert.False(t, checkResult.FabricManagerActive, "Expected FabricManagerActive to be false")
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
	}

	// Create the component with our mock instance
	comp := &component{
		ctx:               context.Background(),
		cancel:            func() {},
		loadNVML:          mockInstance,
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
	}

	// Create component with mock
	c := &component{
		ctx:      context.Background(),
		loadNVML: mockNVML,
	}

	// Call Check
	result := c.Check()

	// Assert on result
	checkResult, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", checkResult.reason)
}
