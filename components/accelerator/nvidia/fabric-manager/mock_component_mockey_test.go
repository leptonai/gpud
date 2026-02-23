package fabricmanager

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCheckFMExists_Mockey(t *testing.T) {
	t.Run("nv-fabricmanager found", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(exec.LookPath).Return("/usr/bin/nv-fabricmanager", nil).Build()
			assert.True(t, checkFMExists())
		})
	})

	t.Run("nv-fabricmanager not found", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(exec.LookPath).Return("", assert.AnError).Build()
			assert.False(t, checkFMExists())
		})
	})
}

func TestCheckFMActive_Mockey(t *testing.T) {
	t.Run("port open", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(netutil.IsPortOpen).Return(true).Build()
			assert.True(t, checkFMActive())
		})
	})

	t.Run("port closed", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(netutil.IsPortOpen).Return(false).Build()
			assert.False(t, checkFMActive())
		})
	})
}

func TestListPCINVSwitches_Mockey(t *testing.T) {
	// Test listPCIs directly with inline NVSwitch bridge data
	data := []byte("0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)")
	script := buildPrintScript(t, data)
	mockey.PatchRun(func() {
		// Mock LocateExecutable to return the script path
		mockey.Mock(file.LocateExecutable).Return(script, nil).Build()
		// Do not mock process.New or process.Read, let them run the script which will output the right data
		ctx := context.Background()
		lines, err := listPCIs(ctx, script, isNVIDIANVSwitchPCI)
		assert.NoError(t, err)
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0], "Bridge")
	})
}

func TestCountSMINVSwitches_Mockey(t *testing.T) {
	// Test countSMINVSwitches directly with inline GPU data
	data := []byte("GPU 0: NVIDIA A100-SXM4-80GB (UUID: GPU-123)\nGPU 1: NVIDIA A100-SXM4-80GB (UUID: GPU-456)")
	script := buildPrintScript(t, data)
	mockey.PatchRun(func() {
		// Mock LocateExecutable to return the script path
		mockey.Mock(file.LocateExecutable).Return(script, nil).Build()
		// Do not mock process.New or process.Read, let them run the script which will output the right data
		ctx := context.Background()
		lines, err := countSMINVSwitches(ctx, script)
		assert.NoError(t, err)
		assert.Len(t, lines, 2)
		assert.Contains(t, lines[0], "GPU 0")
		assert.Contains(t, lines[1], "GPU 1")
	})
}

// TestHasNothingToDoEvent_NilBucket tests hasNothingToDoEvent when eventBucket is nil.
func TestHasNothingToDoEvent_NilBucket(t *testing.T) {
	t.Parallel()

	c := &component{
		ctx:         context.Background(),
		eventBucket: nil,
	}
	assert.False(t, c.hasNothingToDoEvent())
}

// TestHasNothingToDoEvent_WithMatchingEvent tests hasNothingToDoEvent finds a matching event.
func TestHasNothingToDoEvent_WithMatchingEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert a matching nothing-to-do event
	err = bucket.Insert(ctx, eventstore.Event{
		Time:    time.Now().Add(-5 * time.Minute), // within the 10-minute window
		Name:    EventNVSwitchNothingToDo,
		Type:    "Warning",
		Message: messageNVSwitchNothingToDo,
	})
	require.NoError(t, err)

	c := &component{
		ctx:         ctx,
		eventBucket: bucket,
	}
	assert.True(t, c.hasNothingToDoEvent())
}

// TestHasNothingToDoEvent_NoMatchingEvent tests hasNothingToDoEvent when no matching events exist.
func TestHasNothingToDoEvent_NoMatchingEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert a different event (not nothing-to-do)
	err = bucket.Insert(ctx, eventstore.Event{
		Time:    time.Now().Add(-5 * time.Minute),
		Name:    eventNVSwitchNonFatalSXid,
		Type:    "Warning",
		Message: "some other event",
	})
	require.NoError(t, err)

	c := &component{
		ctx:         ctx,
		eventBucket: bucket,
	}
	assert.False(t, c.hasNothingToDoEvent())
}

// TestHasNothingToDoEvent_ErrorQuerying tests hasNothingToDoEvent when the bucket returns an error.
func TestHasNothingToDoEvent_ErrorQuerying(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)

	// Close the bucket to cause Get errors
	bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:         ctx,
		eventBucket: bucket,
	}
	// Should return false when there's an error querying events
	assert.False(t, c.hasNothingToDoEvent())
}

// TestCheck_NVSwitchNotDetected_SkipsFabricState tests that when NVSwitch hardware is not
// detected (e.g., GH200 standalone), the fabric state check is skipped and reported as healthy.
func TestCheck_NVSwitchNotDetected_SkipsFabricState(t *testing.T) {
	t.Parallel()

	fabricStateCalled := false
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GH200",
			deviceCount:         2,
		},
		collectFabricStateFunc: func() fabricStateReport {
			fabricStateCalled = true
			return fabricStateReport{Healthy: true}
		},
		checkNVSwitchExistsFunc: func() bool { return false },
		testingMode:             false,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.False(t, fabricStateCalled, "fabric state collection should be skipped when NVSwitch not detected")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "NVSwitch not detected, skipping fabric state check")
	assert.Equal(t, cr.reason, cr.FabricStateReason)
	assert.True(t, cr.FabricStateSupported)
}

// TestCheck_NVSwitchNotDetected_TestingMode_DoesNotSkip tests that in testing mode,
// the NVSwitch detection is bypassed and fabric state is checked.
func TestCheck_NVSwitchNotDetected_TestingMode_DoesNotSkip(t *testing.T) {
	t.Parallel()

	fabricStateCalled := false
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA H100 80GB HBM3",
			deviceCount:         2,
		},
		collectFabricStateFunc: func() fabricStateReport {
			fabricStateCalled = true
			return fabricStateReport{Healthy: true}
		},
		checkNVSwitchExistsFunc: func() bool { return false },
		testingMode:             true, // testing mode should bypass NVSwitch check
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, fabricStateCalled, "fabric state should be checked in testing mode")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

// TestCheck_FMNotActive_NothingToDoEvent tests that when FM is not active but has
// reported "nothing to do", the component is treated as healthy.
func TestCheck_FMNotActive_NothingToDoEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert a nothing-to-do event
	err = bucket.Insert(ctx, eventstore.Event{
		Time:    time.Now().Add(-2 * time.Minute),
		Name:    EventNVSwitchNothingToDo,
		Type:    "Warning",
		Message: messageNVSwitchNothingToDo,
	})
	require.NoError(t, err)

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			supportsFM:  true,
			productName: "NVIDIA GH200",
			deviceCount: 2,
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return false }, // FM not active
		eventBucket:             bucket,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Should be healthy when FM reports nothing to do")
	assert.Contains(t, cr.reason, "fabric manager has nothing to do (no NVSwitch devices)")
	assert.False(t, cr.FabricManagerActive)
}

// TestCheck_FMNotActive_NoNothingToDoEvent tests that when FM is not active and there
// is no "nothing to do" event, the component is unhealthy.
func TestCheck_FMNotActive_NoNothingToDoEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			supportsFM:  true,
			productName: "NVIDIA H100",
			deviceCount: 2,
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return false },
		eventBucket:             bucket, // empty bucket, no events
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should be unhealthy when FM not active and no nothing-to-do event")
	assert.Contains(t, cr.reason, "fabric manager found but not active")
}

// TestCheck_FabricStateUnhealthy_ThenFMNothingToDo tests that when fabric state is
// unhealthy AND FM reports nothing-to-do, the unhealthy state is preserved.
func TestCheck_FabricStateUnhealthy_ThenFMNothingToDo(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert a nothing-to-do event
	err = bucket.Insert(ctx, eventstore.Event{
		Time:    time.Now().Add(-2 * time.Minute),
		Name:    EventNVSwitchNothingToDo,
		Type:    "Warning",
		Message: messageNVSwitchNothingToDo,
	})
	require.NoError(t, err)

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          true,
			supportsFabricState: true,
			productName:         "NVIDIA H100",
			deviceCount:         2,
		},
		testingMode: true, // bypass NVSwitch check
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Healthy: false,
				Reason:  "bandwidth degraded",
			}
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return false },
		eventBucket:             bucket,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Unhealthy from fabric state should be preserved even with nothing-to-do
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Unhealthy state from fabric state should be preserved")
}

// TestCheck_InitError tests that NVML initialization errors result in unhealthy state.
func TestCheck_InitError(t *testing.T) {
	t.Parallel()

	mockInstance := &mockNVMLInstanceWithInitError{
		mockNVMLInstance: mockNVMLInstance{
			exists:      true,
			productName: "Test GPU",
			deviceCount: 2,
		},
		initErr: errors.New("NVML init failed"),
	}

	comp := &component{
		ctx:          context.Background(),
		cancel:       func() {},
		nvmlInstance: mockInstance,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "NVML initialization error")
	assert.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}

// TestCheck_GetCountLspciError tests the error path in GPU count check.
func TestCheck_GetCountLspciError(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          true,
			supportsFabricState: true,
			productName:         "NVIDIA H100",
			deviceCount:         2,
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 0, errors.New("lspci failed")
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{Healthy: true}
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Despite lspci error, Check should continue and succeed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "fabric manager found and active", cr.reason)
}

// TestAppendReason_BothEmpty tests appendReason with both empty strings.
func TestAppendReason_BothEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", appendReason("", ""))
}

// TestAppendReason_ExistingEmpty tests appendReason with empty existing.
func TestAppendReason_ExistingEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "new reason", appendReason("", "new reason"))
}

// TestAppendReason_AdditionEmpty tests appendReason with empty addition.
func TestAppendReason_AdditionEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "existing", appendReason("existing", ""))
}

// TestAppendReason_BothPresent tests appendReason with both values.
func TestAppendReason_BothPresent(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "existing; addition", appendReason("existing", "addition"))
}

// TestCheck_FabricStateSupported_NVSwitchNil_SkipsFabricState tests that when
// checkNVSwitchExistsFunc is nil and fabric state is supported, fabric state is still collected.
func TestCheck_FabricStateSupported_NVSwitchNil_SkipsFabricState(t *testing.T) {
	t.Parallel()

	fabricStateCalled := false
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200",
			deviceCount:         2,
		},
		collectFabricStateFunc: func() fabricStateReport {
			fabricStateCalled = true
			return fabricStateReport{Healthy: true}
		},
		checkNVSwitchExistsFunc: nil, // nil NVSwitch check
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, fabricStateCalled, "fabric state should be collected when NVSwitch func is nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

// TestCheck_StringOutput tests the String() method for various scenarios.
func TestCheck_StringOutput_FabricState(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		FabricStateSupported: true,
		FabricStateReason:    "test reason",
	}
	output := cr.String()
	assert.Contains(t, output, "fabric manager is not active")
}

// TestCheck_HealthStates_NilCheckResult tests HealthStates on nil checkResult.
func TestCheck_HealthStates_NilCheckResult(t *testing.T) {
	t.Parallel()

	var cr *checkResult
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// TestCheck_HealthStates_WithUnhealthy tests HealthStates with unhealthy state and error.
func TestCheck_HealthStates_WithUnhealthy(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "NVML init error",
		err:    errors.New("init failed"),
	}
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "NVML init error", states[0].Reason)
	assert.Equal(t, "init failed", states[0].Error)
}

// mockNVMLInstanceWithInitError wraps mockNVMLInstance to return an init error.
type mockNVMLInstanceWithInitError struct {
	mockNVMLInstance
	initErr error
}

func (m *mockNVMLInstanceWithInitError) InitError() error {
	return m.initErr
}

func (m *mockNVMLInstanceWithInitError) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstanceWithInitError) ProductName() string {
	return m.productName
}

// TestCheck_FabricStateWithReason tests that FabricStateReason is properly set from report.
func TestCheck_FabricStateWithReason(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200",
			deviceCount:         2,
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Healthy: true,
				Reason:  "all GPUs healthy",
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, "all GPUs healthy", cr.FabricStateReason)
}

// TestComponentEvents_Timestamp verifies event timestamps are parsed correctly.
func TestComponentEvents_Timestamp(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert events at specific times
	now := time.Now().UTC()
	events := []eventstore.Event{
		{
			Time:    now.Add(-5 * time.Minute),
			Name:    EventNVSwitchNothingToDo,
			Type:    "Warning",
			Message: messageNVSwitchNothingToDo,
		},
		{
			Time:    now.Add(-1 * time.Minute),
			Name:    eventNVSwitchNonFatalSXid,
			Type:    "Warning",
			Message: messageNVSwitchNonFatalSXid,
		},
	}

	for _, ev := range events {
		require.NoError(t, bucket.Insert(ctx, ev))
	}

	// Query events and verify count
	storedEvents, err := bucket.Get(ctx, now.Add(-10*time.Minute))
	require.NoError(t, err)
	assert.Len(t, storedEvents, 2)
}

// TestNewWithEventStore tests the New function with a valid eventstore.
func TestNewWithEventStore(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	instance := &components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
	}

	comp, err := New(instance)
	require.NoError(t, err)
	require.NotNil(t, comp)

	err = comp.Close()
	assert.NoError(t, err)
}

// TestCheck_FMNotActive_NothingToDo_WithPriorUnhealthy tests the interaction
// between prior unhealthy fabric state and the nothing-to-do event path.
func TestCheck_FMNotActive_NothingToDo_PreservesUnhealthy(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert a nothing-to-do event
	err = bucket.Insert(ctx, eventstore.Event{
		Time:    time.Now().Add(-2 * time.Minute),
		Name:    EventNVSwitchNothingToDo,
		Type:    "Warning",
		Message: messageNVSwitchNothingToDo,
	})
	require.NoError(t, err)

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          true,
			supportsFabricState: true,
			productName:         "NVIDIA H100",
			deviceCount:         2,
		},
		testingMode: true,
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Healthy: false,
				Reason:  "GPU-0 bandwidth degraded",
			}
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return false }, // FM not active
		eventBucket:             bucket,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// The cr.health should still be unhealthy because fabric state was unhealthy
	// and hasNothingToDoEvent only sets healthy if health != unhealthy
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Unhealthy fabric state should be preserved even when nothing-to-do event exists")
}

// TestDataString_WithFabricStateReason tests String() output includes fabric state info.
func TestDataString_WithFabricStateReason(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		FabricStateSupported: true,
		FabricStates:         nil,
		FabricStateReason:    "all healthy",
		FabricManagerActive:  true,
	}
	output := cr.String()
	assert.Contains(t, output, "fabric manager is active")

	cr2 := &checkResult{
		FabricStateSupported: true,
		FabricManagerActive:  false,
	}
	output2 := cr2.String()
	assert.Contains(t, output2, "fabric manager is not active")
}

// TestCheck_LastHealthStates_AfterMultipleChecks tests that LastHealthStates
// returns the most recent check result.
func TestCheck_LastHealthStates_AfterMultipleChecks(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:                     context.Background(),
		cancel:                  func() {},
		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, productName: "Test GPU", deviceCount: 2},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },
	}

	// First check - healthy
	_ = comp.Check()
	states := comp.LastHealthStates()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Change FM to not active
	comp.checkFMActiveFunc = func() bool { return false }

	// Second check - unhealthy
	_ = comp.Check()
	states = comp.LastHealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "fabric manager found but not active")
}

// TestEventsNilLogLineProcessor verifies Events returns nil, nil when processor is absent.
func TestEventsNilLogLineProcessor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
	}

	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestCheckResult_HealthStates_ExtraInfo tests that check result data is properly
// serialized into the health states ExtraInfo field.
func TestCheckResult_HealthStates_ExtraInfo(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		ts:                   time.Now().UTC(),
		health:               apiv1.HealthStateTypeHealthy,
		reason:               "fabric manager found and active",
		FabricManagerActive:  true,
		FabricStateSupported: true,
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "fabric_manager_active")
}

func buildPrintScript(t *testing.T, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n")
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		buf.WriteString("printf '%s\\n' '")
		buf.WriteString(escapeSingleQuotes(scanner.Text()))
		buf.WriteString("'\n")
	}
	require.NoError(t, scanner.Err())
	scriptPath := filepath.Join(t.TempDir(), "emit.sh")
	require.NoError(t, os.WriteFile(scriptPath, buf.Bytes(), 0o755))
	return scriptPath
}

func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\"'\"'")
}
