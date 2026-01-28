package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/components/all"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
)

// mockComponent implements components.Component for testing
type mockComponent struct {
	name        string
	supported   bool
	checkResult components.CheckResult
}

func (m *mockComponent) Name() string                         { return m.name }
func (m *mockComponent) Tags() []string                       { return nil }
func (m *mockComponent) IsSupported() bool                    { return m.supported }
func (m *mockComponent) Start() error                         { return nil }
func (m *mockComponent) Check() components.CheckResult        { return m.checkResult }
func (m *mockComponent) LastHealthStates() apiv1.HealthStates { return nil }
func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}
func (m *mockComponent) Close() error { return nil }

// TestScan_NVMLInstanceCreationError tests error handling when NVML instance creation fails.
func TestScan_NVMLInstanceCreationError(t *testing.T) {
	mockey.PatchConvey("nvml instance creation error", t, func() {
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return nil, errors.New("nvml library not found")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nvml library not found")
	})
}

// TestScan_GetMachineInfoError tests error handling when GetMachineInfo fails.
func TestScan_GetMachineInfoError(t *testing.T) {
	mockey.PatchConvey("get machine info error", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return nil, errors.New("failed to get CPU info")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get CPU info")
	})
}

// TestScan_ComponentInitError tests error handling when component initialization fails.
func TestScan_ComponentInitError(t *testing.T) {
	mockey.PatchConvey("component init error", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil, // No GPU info
			}, nil
		}).Build()

		// Mock all.All() to return a component that fails to initialize
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "test-failing-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return nil, errors.New("component initialization failed")
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component initialization failed")
	})
}

// TestScan_Success tests successful scan execution.
func TestScan_Success(t *testing.T) {
	mockey.PatchConvey("successful scan", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Mock all.All() to return an empty list (no components to check)
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
	})
}

// TestScan_SuccessWithGPUInfo tests successful scan with GPU info present.
func TestScan_SuccessWithGPUInfo(t *testing.T) {
	mockey.PatchConvey("successful scan with GPU info", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo: &apiv1.MachineGPUInfo{
					Product: "NVIDIA H100 80GB HBM3",
				},
			}, nil
		}).Build()

		mockey.Mock(infiniband.SupportsInfinibandPortRate).To(func(gpuProductName string) (types.ExpectedPortStates, error) {
			return types.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}, nil
		}).Build()

		setDefaultCalled := false
		mockey.Mock(infiniband.SetDefaultExpectedPortStates).To(func(states types.ExpectedPortStates) {
			setDefaultCalled = true
			assert.Equal(t, 8, states.AtLeastPorts)
			assert.Equal(t, 400, states.AtLeastRate)
		}).Build()

		// Mock all.All() to return an empty list
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		assert.True(t, setDefaultCalled, "SetDefaultExpectedPortStates should have been called")
	})
}

// TestScan_UnsupportedGPUForInfiniband tests scan with GPU that doesn't support InfiniBand.
func TestScan_UnsupportedGPUForInfiniband(t *testing.T) {
	mockey.PatchConvey("unsupported GPU for infiniband", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo: &apiv1.MachineGPUInfo{
					Product: "GeForce RTX 4090", // Consumer GPU without InfiniBand
				},
			}, nil
		}).Build()

		mockey.Mock(infiniband.SupportsInfinibandPortRate).To(func(gpuProductName string) (types.ExpectedPortStates, error) {
			return types.ExpectedPortStates{}, infiniband.ErrNoExpectedPortStates
		}).Build()

		setDefaultCalled := false
		mockey.Mock(infiniband.SetDefaultExpectedPortStates).To(func(states types.ExpectedPortStates) {
			setDefaultCalled = true
		}).Build()

		// Mock all.All() to return an empty list
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		assert.False(t, setDefaultCalled, "SetDefaultExpectedPortStates should not have been called for unsupported GPU")
	})
}

// TestScan_WithSupportedComponent tests scan with a supported component.
func TestScan_WithSupportedComponent(t *testing.T) {
	mockey.PatchConvey("scan with supported component", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Create a mock component
		mockComp := &mockComponent{
			name:      "test-component",
			supported: true,
			checkResult: &mockCheckResult{
				componentName:   "test-component",
				summary:         "All systems operational",
				healthStateType: apiv1.HealthStateTypeHealthy,
				stringOutput:    "Test output",
			},
		}

		// Mock all.All() to return our mock component
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "test-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return mockComp, nil
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
	})
}

// TestScan_WithUnsupportedComponent tests scan with an unsupported component (should be skipped).
func TestScan_WithUnsupportedComponent(t *testing.T) {
	mockey.PatchConvey("scan with unsupported component", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Create a mock component that is not supported
		mockComp := &mockComponent{
			name:      "test-unsupported-component",
			supported: false,
		}

		// Mock all.All() to return our unsupported component
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "test-unsupported-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return mockComp, nil
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
	})
}

// TestScan_WithUnhealthyComponent tests scan with an unhealthy component.
func TestScan_WithUnhealthyComponent(t *testing.T) {
	mockey.PatchConvey("scan with unhealthy component", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Create a mock component with unhealthy check result
		mockComp := &mockComponent{
			name:      "test-unhealthy-component",
			supported: true,
			checkResult: &mockCheckResult{
				componentName:   "test-unhealthy-component",
				summary:         "Hardware error detected",
				healthStateType: apiv1.HealthStateTypeUnhealthy,
				stringOutput:    "GPU temperature exceeded threshold",
			},
		}

		// Mock all.All() to return our unhealthy component
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "test-unhealthy-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return mockComp, nil
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
	})
}

// TestScan_WithFailureInjector tests scan with failure injector configured.
func TestScan_WithFailureInjector(t *testing.T) {
	mockey.PatchConvey("scan with failure injector", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		// Should use NewWithFailureInjector when failure injector has GPU-related config
		newWithFailureInjectorCalled := false
		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			newWithFailureInjectorCalled = true
			assert.Contains(t, config.GPUUUIDsWithGPULost, "GPU-123")
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Mock all.All() to return an empty list
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUUUIDsWithGPULost: []string{"GPU-123"},
		}))
		require.NoError(t, err)
		assert.True(t, newWithFailureInjectorCalled, "NewWithFailureInjector should have been called")
	})
}

// TestScan_WithFailureInjectorError tests error handling when NewWithFailureInjector fails.
func TestScan_WithFailureInjectorError(t *testing.T) {
	mockey.PatchConvey("failure injector creation error", t, func() {
		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			return nil, errors.New("failed to create failure injector instance")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUUUIDsWithGPULost: []string{"GPU-123"},
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create failure injector instance")
	})
}

// TestScan_WithGPURequiresResetInjection tests scan with GPU requires reset failure injection.
func TestScan_WithGPURequiresResetInjection(t *testing.T) {
	mockey.PatchConvey("scan with GPU requires reset injection", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			assert.Contains(t, config.GPUUUIDsWithGPURequiresReset, "GPU-456")
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUUUIDsWithGPURequiresReset: []string{"GPU-456"},
		}))
		require.NoError(t, err)
	})
}

// TestScan_WithFabricStateUnhealthyInjection tests scan with fabric state unhealthy injection.
func TestScan_WithFabricStateUnhealthyInjection(t *testing.T) {
	mockey.PatchConvey("scan with fabric state unhealthy injection", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			assert.Contains(t, config.GPUUUIDsWithFabricStateHealthSummaryUnhealthy, "GPU-789")
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: []string{"GPU-789"},
		}))
		require.NoError(t, err)
	})
}

// TestScan_WithProductNameOverride tests scan with GPU product name override.
func TestScan_WithProductNameOverride(t *testing.T) {
	mockey.PatchConvey("scan with product name override", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			assert.Equal(t, "NVIDIA H100 SXM", config.GPUProductNameOverride)
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUProductNameOverride: "NVIDIA H100 SXM",
		}))
		require.NoError(t, err)
	})
}

// TestScan_WithInfinibandClassRootDir tests scan with custom InfiniBand class root dir.
func TestScan_WithInfinibandClassRootDir(t *testing.T) {
	mockey.PatchConvey("scan with custom infiniband class root dir", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithInfinibandClassRootDir("/custom/infiniband/path"))
		require.NoError(t, err)
	})
}

// TestScan_WithMultipleComponents tests scan with multiple components.
func TestScan_WithMultipleComponents(t *testing.T) {
	mockey.PatchConvey("scan with multiple components", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		// Create multiple mock components
		healthyComp := &mockComponent{
			name:      "healthy-component",
			supported: true,
			checkResult: &mockCheckResult{
				componentName:   "healthy-component",
				summary:         "Healthy",
				healthStateType: apiv1.HealthStateTypeHealthy,
				stringOutput:    "All good",
			},
		}
		degradedComp := &mockComponent{
			name:      "degraded-component",
			supported: true,
			checkResult: &mockCheckResult{
				componentName:   "degraded-component",
				summary:         "Degraded",
				healthStateType: apiv1.HealthStateTypeDegraded,
				stringOutput:    "Performance reduced",
			},
		}
		unsupportedComp := &mockComponent{
			name:      "unsupported-component",
			supported: false,
		}

		// Mock all.All() to return multiple components
		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "healthy-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return healthyComp, nil
					},
				},
				{
					Name: "degraded-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return degradedComp, nil
					},
				},
				{
					Name: "unsupported-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return unsupportedComp, nil
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
	})
}

// TestScan_EmptyGPUProductName tests scan when GPU info has empty product name.
func TestScan_EmptyGPUProductName(t *testing.T) {
	mockey.PatchConvey("empty GPU product name", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo: &apiv1.MachineGPUInfo{
					Product: "", // Empty product name
				},
			}, nil
		}).Build()

		setDefaultCalled := false
		mockey.Mock(infiniband.SetDefaultExpectedPortStates).To(func(states types.ExpectedPortStates) {
			setDefaultCalled = true
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		// SetDefaultExpectedPortStates should not be called when product name is empty
		assert.False(t, setDefaultCalled)
	})
}

// TestScan_NilGPUInfo tests scan when GPU info is nil.
func TestScan_NilGPUInfo(t *testing.T) {
	mockey.PatchConvey("nil GPU info", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil, // No GPU info
			}, nil
		}).Build()

		setDefaultCalled := false
		mockey.Mock(infiniband.SetDefaultExpectedPortStates).To(func(states types.ExpectedPortStates) {
			setDefaultCalled = true
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		// SetDefaultExpectedPortStates should not be called when GPU info is nil
		assert.False(t, setDefaultCalled)
	})
}

// TestScan_NoFailureInjectorUsesRegularNew tests that regular nvidianvml.New is used when no failure injector config.
func TestScan_NoFailureInjectorUsesRegularNew(t *testing.T) {
	mockey.PatchConvey("no failure injector uses regular New", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		regularNewCalled := false
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			regularNewCalled = true
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		assert.True(t, regularNewCalled, "regular nvidianvml.New should have been called")
	})
}

// TestScan_FailureInjectorWithEmptyFields tests that regular New is used when failure injector has no GPU-related fields.
func TestScan_FailureInjectorWithEmptyFields(t *testing.T) {
	mockey.PatchConvey("failure injector with empty fields uses regular New", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		regularNewCalled := false
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			regularNewCalled = true
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Empty failure injector should not trigger NewWithFailureInjector
		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{}))
		require.NoError(t, err)
		assert.True(t, regularNewCalled, "regular nvidianvml.New should have been called for empty failure injector")
	})
}

// TestScan_A100GPU tests scan with A100 GPU.
func TestScan_A100GPU(t *testing.T) {
	mockey.PatchConvey("scan with A100 GPU", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo: &apiv1.MachineGPUInfo{
					Product: "NVIDIA A100-SXM4-40GB",
				},
			}, nil
		}).Build()

		mockey.Mock(infiniband.SupportsInfinibandPortRate).To(func(gpuProductName string) (types.ExpectedPortStates, error) {
			return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 200}, nil
		}).Build()

		var capturedStates types.ExpectedPortStates
		mockey.Mock(infiniband.SetDefaultExpectedPortStates).To(func(states types.ExpectedPortStates) {
			capturedStates = states
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, capturedStates.AtLeastPorts)
		assert.Equal(t, 200, capturedStates.AtLeastRate)
	})
}

// TestScan_ContextCancellation tests that scan respects context cancellation.
func TestScan_ContextCancellation(t *testing.T) {
	mockey.PatchConvey("context cancellation", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := Scan(ctx)
		// Scan should still complete since context is only used in the function
		require.NoError(t, err)
	})
}

// TestScan_WithCombinedFailureInjectorFields tests scan with multiple failure injector fields set simultaneously.
func TestScan_WithCombinedFailureInjectorFields(t *testing.T) {
	mockey.PatchConvey("scan with combined failure injector fields", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			assert.Contains(t, config.GPUUUIDsWithGPULost, "GPU-111")
			assert.Contains(t, config.GPUUUIDsWithGPURequiresReset, "GPU-222")
			assert.Contains(t, config.GPUUUIDsWithFabricStateHealthSummaryUnhealthy, "GPU-333")
			assert.Equal(t, "NVIDIA H100 SXM", config.GPUProductNameOverride)
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithFailureInjector(&components.FailureInjector{
			GPUUUIDsWithGPULost:                           []string{"GPU-111"},
			GPUUUIDsWithGPURequiresReset:                  []string{"GPU-222"},
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: []string{"GPU-333"},
			GPUProductNameOverride:                        "NVIDIA H100 SXM",
		}))
		require.NoError(t, err)
	})
}

// TestScan_ComponentInitErrorAfterSuccess tests that scan stops when a component init fails after a previous one succeeded.
func TestScan_ComponentInitErrorAfterSuccess(t *testing.T) {
	mockey.PatchConvey("component init error after success", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		successComp := &mockComponent{
			name:      "success-component",
			supported: true,
			checkResult: &mockCheckResult{
				componentName:   "success-component",
				summary:         "OK",
				healthStateType: apiv1.HealthStateTypeHealthy,
				stringOutput:    "All good",
			},
		}

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{
				{
					Name: "success-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return successComp, nil
					},
				},
				{
					Name: "failing-component",
					InitFunc: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
						return nil, errors.New("second component failed")
					},
				},
			}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "second component failed")
	})
}

// TestScan_WithDebugOption tests scan with debug option enabled.
func TestScan_WithDebugOption(t *testing.T) {
	mockey.PatchConvey("scan with debug option", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx, WithDebug(true))
		require.NoError(t, err)
	})
}

// TestScan_WithAllOptions tests scan with all options combined.
func TestScan_WithAllOptions(t *testing.T) {
	mockey.PatchConvey("scan with all options combined", t, func() {
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(nvidianvml.NewWithFailureInjector).To(func(config *nvidianvml.FailureInjectorConfig) (nvidianvml.Instance, error) {
			assert.Contains(t, config.GPUUUIDsWithGPULost, "GPU-ALL")
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				GPUInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(all.All).To(func() []all.Component {
			return []all.Component{}
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := Scan(ctx,
			WithDebug(true),
			WithInfinibandClassRootDir("/custom/ib/path"),
			WithFailureInjector(&components.FailureInjector{
				GPUUUIDsWithGPULost: []string{"GPU-ALL"},
			}),
		)
		require.NoError(t, err)
	})
}

// TestOp_ApplyOpts tests the Op.applyOpts function.
func TestOp_ApplyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, op.infinibandClassRootDir)
	})

	t.Run("with infiniband class root dir", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithInfinibandClassRootDir("/custom/path")})
		require.NoError(t, err)
		assert.Equal(t, "/custom/path", op.infinibandClassRootDir)
	})

	t.Run("with debug", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithDebug(true)})
		require.NoError(t, err)
		assert.True(t, op.debug)
	})

	t.Run("with failure injector", func(t *testing.T) {
		op := &Op{}
		injector := &components.FailureInjector{
			GPUUUIDsWithGPULost: []string{"GPU-123"},
		}
		err := op.applyOpts([]OpOption{WithFailureInjector(injector)})
		require.NoError(t, err)
		assert.Equal(t, injector, op.failureInjector)
	})

	t.Run("with multiple options", func(t *testing.T) {
		op := &Op{}
		injector := &components.FailureInjector{
			GPUUUIDsWithGPULost: []string{"GPU-123"},
		}
		err := op.applyOpts([]OpOption{
			WithInfinibandClassRootDir("/custom/path"),
			WithDebug(true),
			WithFailureInjector(injector),
		})
		require.NoError(t, err)
		assert.Equal(t, "/custom/path", op.infinibandClassRootDir)
		assert.True(t, op.debug)
		assert.Equal(t, injector, op.failureInjector)
	})
}
