//go:build linux

package peermem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/process"
)

// customMockNVMLInstancePeermem with customizable NVMLExists and ProductName
type customMockNVMLInstancePeermem struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initErr     error
}

func (m *customMockNVMLInstancePeermem) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstancePeermem) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstancePeermem) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstancePeermem) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstancePeermem) ProductName() string   { return m.productName }
func (m *customMockNVMLInstancePeermem) Architecture() string  { return "" }
func (m *customMockNVMLInstancePeermem) Brand() string         { return "" }
func (m *customMockNVMLInstancePeermem) DriverVersion() string { return "" }
func (m *customMockNVMLInstancePeermem) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstancePeermem) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstancePeermem) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstancePeermem) Library() lib.Library  { return nil }
func (m *customMockNVMLInstancePeermem) Shutdown() error       { return nil }
func (m *customMockNVMLInstancePeermem) InitError() error      { return m.initErr }

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstancePeermem{
			devs:        nil,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, Name, c.Name())

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.checkLsmodPeermemModuleFunc)
	})
}

// TestComponent_IsSupportedWithMockey tests IsSupported method with various conditions using mockey
func TestComponent_IsSupportedWithMockey(t *testing.T) {
	testCases := []struct {
		name         string
		setupNilNVML bool
		nvmlExists   bool
		productName  string
		expected     bool
	}{
		{
			name:         "nil NVML instance returns false",
			setupNilNVML: true,
			expected:     false,
		},
		{
			name:        "NVML not loaded returns false",
			nvmlExists:  false,
			productName: "NVIDIA H100",
			expected:    false,
		},
		{
			name:        "no product name returns false",
			nvmlExists:  true,
			productName: "",
			expected:    false,
		},
		{
			name:        "NVML loaded with product name returns true",
			nvmlExists:  true,
			productName: "NVIDIA H100",
			expected:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				ctx := context.Background()
				cctx, cancel := context.WithCancel(ctx)
				defer cancel()

				var comp *component
				if tc.setupNilNVML {
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: nil,
					}
				} else {
					customMock := &customMockNVMLInstancePeermem{
						devs:        map[string]device.Device{},
						nvmlExists:  tc.nvmlExists,
						productName: tc.productName,
					}
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: customMock,
					}
				}

				result := comp.IsSupported()
				assert.Equal(t, tc.expected, result)
			})
		})
	}
}

// TestCheck_InitError tests Check when NVML has an initialization error
func TestCheck_InitErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initErr:     initErr,
		}

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: mockInst,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "NVML initialization error")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_MissingProductNameWithMockey tests Check when product name is empty
func TestCheck_MissingProductNameWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: mockInst,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "missing product name")
	})
}

// TestCheck_NilNVMLInstanceWithMockey tests Check with nil NVML instance
func TestCheck_NilNVMLInstanceWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with nil NVML instance", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: nil,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML instance is nil")
	})
}

// TestCheck_NVMLNotExistsWithMockey tests Check when NVML library is not loaded
func TestCheck_NVMLNotExistsWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML not exists", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: mockInst,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheck_ConcurrentAccessWithMockey tests concurrent access to Check and LastHealthStates
func TestCheck_ConcurrentAccessWithMockey(t *testing.T) {
	mockey.PatchConvey("Concurrent Check and LastHealthStates access", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			return &LsmodPeermemModuleOutput{
				Raw:                      "ib_core 123456 1 nvidia_peermem",
				IbcoreUsingPeermemModule: true,
			}, nil
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		// Run concurrent access
		done := make(chan bool, 10)
		for i := 0; i < 5; i++ {
			go func() {
				comp.Check()
				done <- true
			}()
			go func() {
				_ = comp.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state is consistent
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
	})
}

// TestCheckLsmodPeermemModule_NonRootWithMockey tests CheckLsmodPeermemModule when not running as root
func TestCheckLsmodPeermemModule_NonRootWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule non-root", t, func() {
		result, err := checkLsmodPeermemModule(
			context.Background(),
			func() int { return 1000 },
			process.New,
		)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "requires sudo/root access")
	})
}

// TestCheckLsmodPeermemModule_ProcessNewErrorWithMockey tests CheckLsmodPeermemModule when process.New fails
func TestCheckLsmodPeermemModule_ProcessNewErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule process.New error", t, func() {
		result, err := checkLsmodPeermemModule(
			context.Background(),
			func() int { return 0 },
			func(opts ...process.OpOption) (process.Process, error) {
				return nil, errors.New("failed to create process")
			},
		)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to create process")
	})
}

// TestCheckResult_MethodsWithMockey tests all checkResult methods
func TestCheckResult_MethodsWithMockey(t *testing.T) {
	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("HealthStates with suggested actions", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "NVML initialization error",
			err:    errors.New("device error"),
			suggestedActions: &apiv1.SuggestedActions{
				Description: "NVML init failed",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})

	t.Run("getError with nil checkResult", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError with nil error", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError with error", func(t *testing.T) {
		cr := &checkResult{err: errors.New("test error")}
		assert.Equal(t, "test error", cr.getError())
	})
}

// TestCheckResult_StringWithMockey tests the String method of checkResult
func TestCheckResult_StringWithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("nil PeerMemModuleOutput", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with PeerMemModuleOutput - peermem loaded", func(t *testing.T) {
		cr := &checkResult{
			PeerMemModuleOutput: &LsmodPeermemModuleOutput{
				IbcoreUsingPeermemModule: true,
			},
		}
		result := cr.String()
		assert.Contains(t, result, "ibcore using peermem module: true")
	})

	t.Run("with PeerMemModuleOutput - peermem not loaded", func(t *testing.T) {
		cr := &checkResult{
			PeerMemModuleOutput: &LsmodPeermemModuleOutput{
				IbcoreUsingPeermemModule: false,
			},
		}
		result := cr.String()
		assert.Contains(t, result, "ibcore using peermem module: false")
	})
}

// TestCheckResult_SummaryWithMockey tests the Summary method
func TestCheckResult_SummaryWithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{reason: "ibcore successfully loaded peermem module"}
		assert.Equal(t, "ibcore successfully loaded peermem module", cr.Summary())
	})
}

// TestCheckResult_HealthStateTypeWithMockey tests the HealthStateType method
func TestCheckResult_HealthStateTypeWithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("unhealthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeUnhealthy}
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})
}

// TestCheckResult_HealthStates_NilResultWithMockey tests HealthStates with nil checkResult
func TestCheckResult_HealthStates_NilResultWithMockey(t *testing.T) {
	var cr *checkResult
	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// TestCheckResult_HealthStates_WithExtraInfoWithMockey tests HealthStates with PeerMemModuleOutput
func TestCheckResult_HealthStates_WithExtraInfoWithMockey(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "ibcore successfully loaded peermem module",
		PeerMemModuleOutput: &LsmodPeermemModuleOutput{
			Raw:                      "ib_core 123456 1 nvidia_peermem",
			IbcoreUsingPeermemModule: true,
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "ibcore_using_peermem_module")
}

// TestCheck_LsmodErrorWithMockey tests Check when checkLsmodPeermemModuleFunc returns an error
func TestCheck_LsmodErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with lsmod error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			return nil, errors.New("lsmod command failed")
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error checking peermem")
		assert.NotNil(t, cr.err)
	})
}

// TestCheck_PeermemLoadedWithMockey tests Check when peermem is successfully loaded
func TestCheck_PeermemLoadedWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with peermem loaded", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			return &LsmodPeermemModuleOutput{
				Raw:                      "ib_core 434176 9 rdma_cm,ib_ipoib,nvidia_peermem",
				IbcoreUsingPeermemModule: true,
			}, nil
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "ibcore successfully loaded peermem module")
		assert.NotNil(t, cr.PeerMemModuleOutput)
		assert.True(t, cr.PeerMemModuleOutput.IbcoreUsingPeermemModule)
	})
}

// TestCheck_PeermemNotLoadedWithMockey tests Check when peermem is not loaded
func TestCheck_PeermemNotLoadedWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with peermem not loaded", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			return &LsmodPeermemModuleOutput{
				Raw:                      "ib_core 434176 9 rdma_cm,ib_ipoib",
				IbcoreUsingPeermemModule: false,
			}, nil
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "ibcore is not using peermem module")
		assert.NotNil(t, cr.PeerMemModuleOutput)
		assert.False(t, cr.PeerMemModuleOutput.IbcoreUsingPeermemModule)
	})
}

// TestCheck_NilPeermemOutputWithMockey tests Check when checkLsmodPeermemModuleFunc returns nil output
func TestCheck_NilPeermemOutputWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with nil peermem output", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			return nil, nil
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "ibcore is not using peermem module")
	})
}

// TestHasLsmodInfinibandPeerMem_EdgeCasesWithMockey tests edge cases for HasLsmodInfinibandPeerMem
func TestHasLsmodInfinibandPeerMem_EdgeCasesWithMockey(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "ib_core with peermem in used modules",
			input:    "ib_core 434176 9 rdma_cm,ib_ipoib,nvidia_peermem,iw_cm,ib_umad,rdma_ucm,ib_uverbs,mlx5_ib,ib_cm",
			expected: true,
		},
		{
			name:     "ib_core without peermem in used modules",
			input:    "ib_core 434176 9 rdma_cm,ib_ipoib,iw_cm,ib_umad,rdma_ucm,ib_uverbs,mlx5_ib,ib_cm",
			expected: false,
		},
		{
			name:     "ib_core with zero usage count",
			input:    "ib_core 434176 0 nvidia_peermem",
			expected: false,
		},
		{
			name:     "other module containing peermem",
			input:    "nvidia 56717312 447 nvidia_uvm,nvidia_peermem,nvidia_modeset",
			expected: false,
		},
		{
			name:     "malformed line with less than 4 fields",
			input:    "ib_core 434176 9",
			expected: false,
		},
		{
			name:     "multiple lines with ib_core having peermem",
			input:    "nvidia_peermem 16384 0\nib_core 434176 9 rdma_cm,nvidia_peermem\nnvidia 56717312 447 nvidia_uvm",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := HasLsmodInfinibandPeerMem(tc.input)
			assert.Equal(t, tc.expected, result, "HasLsmodInfinibandPeerMem(%q)", tc.input)
		})
	}
}

// TestLastHealthStates_SuggestedActionsPropagateWithMockey tests suggested actions propagation
func TestLastHealthStates_SuggestedActionsPropagateWithMockey(t *testing.T) {
	mockey.PatchConvey("Suggested actions propagate to health states", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initErr:     initErr,
		}

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: mockInst,
		}

		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestComponentTags tests the Tags method
func TestComponentTagsWithMockey(t *testing.T) {
	mockey.PatchConvey("Component tags", t, func() {
		comp := &component{}
		tags := comp.Tags()

		assert.Contains(t, tags, "accelerator")
		assert.Contains(t, tags, "gpu")
		assert.Contains(t, tags, "nvidia")
		assert.Contains(t, tags, Name)
	})
}

// TestStartAndCloseWithMockey tests Start and Close methods
func TestStartAndCloseWithMockey(t *testing.T) {
	mockey.PatchConvey("Start and Close", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:          cctx,
			cancel:       cancel,
			nvmlInstance: mockInst,
			checkLsmodPeermemModuleFunc: func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
				return &LsmodPeermemModuleOutput{
					Raw:                      "ib_core 434176 9 nvidia_peermem",
					IbcoreUsingPeermemModule: true,
				}, nil
			},
		}

		err := comp.Start()
		assert.NoError(t, err)

		// Give the goroutine a moment to start
		time.Sleep(50 * time.Millisecond)

		err = comp.Close()
		assert.NoError(t, err)
	})
}

// TestEventsWithMockey tests the Events method with various scenarios
func TestEventsWithMockey(t *testing.T) {
	mockey.PatchConvey("Events method tests", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		t.Run("nil eventBucket returns nil events", func(t *testing.T) {
			comp := &component{
				ctx:         cctx,
				cancel:      cancel,
				eventBucket: nil,
			}

			events, err := comp.Events(context.Background(), time.Now().Add(-time.Hour))
			assert.NoError(t, err)
			assert.Nil(t, events)
		})
	})
}

// TestCheckResultHealthStates_WithSuggestedActions tests HealthStates method with suggested actions
func TestCheckResultHealthStates_WithSuggestedActionsWithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates with suggested actions", t, func() {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "NVML initialization error",
			err:    errors.New("device error"),
			suggestedActions: &apiv1.SuggestedActions{
				Description: "NVML init failed",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)

		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "NVML initialization error", states[0].Reason)
		assert.Equal(t, "device error", states[0].Error)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheckResult_HealthStates_NoSuggestedActionsWithMockey tests HealthStates without suggested actions
func TestCheckResult_HealthStates_NoSuggestedActionsWithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates without suggested actions", t, func() {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "ibcore successfully loaded peermem module",
			PeerMemModuleOutput: &LsmodPeermemModuleOutput{
				Raw:                      "ib_core 434176 9 nvidia_peermem",
				IbcoreUsingPeermemModule: true,
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)

		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "ibcore successfully loaded peermem module", states[0].Reason)
		assert.Empty(t, states[0].Error)
		assert.Nil(t, states[0].SuggestedActions)
	})
}

// TestCheck_ContextCancellationWithMockey tests Check behavior when context is canceled
func TestCheck_ContextCancellationWithMockey(t *testing.T) {
	mockey.PatchConvey("Check with context cancellation", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		checkerCalled := false
		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			checkerCalled = true
			return nil, ctx.Err()
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
		}

		// Cancel context before check
		cancel()

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// The check should still run, but the checker function will receive canceled context
		assert.True(t, checkerCalled)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	})
}

// TestComponentLastHealthStatesRaceCondition tests for race conditions in LastHealthStates
func TestComponentLastHealthStatesRaceConditionWithMockey(t *testing.T) {
	mockey.PatchConvey("LastHealthStates race condition", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstancePeermem{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		var callCount atomic.Int32
		mockChecker := func(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
			callCount.Add(1)
			return &LsmodPeermemModuleOutput{
				Raw:                      "ib_core 434176 9 nvidia_peermem",
				IbcoreUsingPeermemModule: true,
			}, nil
		}

		comp := &component{
			ctx:                         cctx,
			cancel:                      cancel,
			nvmlInstance:                mockInst,
			checkLsmodPeermemModuleFunc: mockChecker,
			lastMu:                      sync.RWMutex{},
		}

		// Run multiple concurrent reads and writes
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				comp.Check()
			}()
			go func() {
				defer wg.Done()
				_ = comp.LastHealthStates()
			}()
		}
		wg.Wait()

		// Should not panic due to race conditions
		assert.True(t, callCount.Load() > 0)
	})
}
