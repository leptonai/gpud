package kubelet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/netutil"
)

// TestCheckKubeletInstalled_Found tests checkKubeletInstalled when kubelet is found
func TestCheckKubeletInstalled_Found(t *testing.T) {
	mockey.PatchConvey("kubelet found in PATH", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			if bin == "kubelet" {
				return "/usr/bin/kubelet", nil
			}
			return "", errors.New("not found")
		}).Build()

		result := checkKubeletInstalled()
		assert.True(t, result)
	})
}

// TestCheckKubeletInstalled_NotFound tests checkKubeletInstalled when kubelet is not found
func TestCheckKubeletInstalled_NotFound(t *testing.T) {
	mockey.PatchConvey("kubelet not found in PATH", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("executable \"kubelet\" not found in PATH")
		}).Build()

		result := checkKubeletInstalled()
		assert.False(t, result)
	})
}

// TestNew_Success tests successful component creation
func TestNew_Success(t *testing.T) {
	mockey.PatchConvey("successful New component creation", t, func() {
		ctx := context.Background()
		gpudInstance := &components.GPUdInstance{
			RootCtx: ctx,
		}

		comp, err := New(gpudInstance)

		require.NoError(t, err)
		require.NotNil(t, comp)

		c, ok := comp.(*component)
		require.True(t, ok)
		assert.NotNil(t, c.ctx)
		assert.NotNil(t, c.cancel)
		assert.NotNil(t, c.checkKubeletInstalled)
		assert.NotNil(t, c.checkKubeletRunning)
		assert.Equal(t, DefaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)
		assert.Equal(t, defaultFailedCountThreshold, c.failedCountThreshold)
		assert.Equal(t, int32(0), c.failedCount.Load())
	})
}

// TestNew_WithMockedDependencies tests New with mocked checkKubeletInstalled
func TestNew_WithMockedDependencies(t *testing.T) {
	mockey.PatchConvey("New with mocked dependencies", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/kubelet", nil
		}).Build()

		mockey.Mock(netutil.IsPortOpen).To(func(port int) bool {
			return true
		}).Build()

		ctx := context.Background()
		gpudInstance := &components.GPUdInstance{
			RootCtx: ctx,
		}

		comp, err := New(gpudInstance)
		require.NoError(t, err)

		c, ok := comp.(*component)
		require.True(t, ok)

		// Verify checkKubeletInstalled function works
		assert.True(t, c.checkKubeletInstalled())
		// Verify checkKubeletRunning function works
		assert.True(t, c.checkKubeletRunning())
	})
}

// TestComponent_Check_KubeletNotInstalled tests Check when kubelet is not installed
func TestComponent_Check_KubeletNotInstalled(t *testing.T) {
	mockey.PatchConvey("Check with kubelet not installed", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return false },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "kubelet is not installed")
		assert.Nil(t, cr.err)
	})
}

// TestComponent_Check_KubeletNotRunning tests Check when kubelet is installed but not running
func TestComponent_Check_KubeletNotRunning(t *testing.T) {
	mockey.PatchConvey("Check with kubelet not running", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return false },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "kubelet is installed but not running")
		assert.Nil(t, cr.err)
	})
}

// TestComponent_Check_NilCheckFunctions tests Check with nil check functions
func TestComponent_Check_NilCheckFunctions(t *testing.T) {
	mockey.PatchConvey("Check with nil checkKubeletInstalled", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: nil,
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "kubelet is not installed")
	})

	mockey.PatchConvey("Check with nil checkKubeletRunning", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   nil,
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "kubelet is installed but not running")
	})
}

// TestComponent_Check_ListPodsError tests Check when ListPodsFromKubeletReadOnlyPort fails
func TestComponent_Check_ListPodsError(t *testing.T) {
	mockey.PatchConvey("Check with ListPodsFromKubeletReadOnlyPort error", t, func() {
		mockey.Mock(ListPodsFromKubeletReadOnlyPort).To(func(ctx context.Context, port int) (string, []PodStatus, error) {
			return "", nil, errors.New("connection refused")
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// First failure should not yet be unhealthy (below threshold)
		assert.NotEqual(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, int32(1), c.failedCount.Load())
	})
}

// TestComponent_Check_FailedCountThreshold tests Check when failed count exceeds threshold
func TestComponent_Check_FailedCountThreshold(t *testing.T) {
	mockey.PatchConvey("Check with failed count at threshold", t, func() {
		mockey.Mock(ListPodsFromKubeletReadOnlyPort).To(func(ctx context.Context, port int) (string, []PodStatus, error) {
			return "", nil, errors.New("connection refused")
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}
		c.failedCount.Store(defaultFailedCountThreshold - 1) // One below threshold

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "list pods from kubelet read-only port failed")
		assert.Equal(t, defaultFailedCountThreshold, c.failedCount.Load())
	})
}

// TestComponent_Check_FailedCountReset tests that failed count resets on success
func TestComponent_Check_FailedCountReset(t *testing.T) {
	mockey.PatchConvey("Check resets failed count on success", t, func() {
		mockey.Mock(ListPodsFromKubeletReadOnlyPort).To(func(ctx context.Context, port int) (string, []PodStatus, error) {
			return "test-node", []PodStatus{{Name: "pod1"}}, nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}
		c.failedCount.Store(3) // Had some previous failures

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, int32(0), c.failedCount.Load()) // Reset to 0
		assert.Equal(t, "test-node", cr.NodeName)
	})
}

// TestComponent_Check_Success tests successful Check
func TestComponent_Check_Success(t *testing.T) {
	mockey.PatchConvey("successful Check", t, func() {
		pods := []PodStatus{
			{
				Name:      "test-pod-1",
				Namespace: "default",
				Phase:     "Running",
			},
			{
				Name:      "test-pod-2",
				Namespace: "kube-system",
				Phase:     "Running",
			},
		}

		mockey.Mock(ListPodsFromKubeletReadOnlyPort).To(func(ctx context.Context, port int) (string, []PodStatus, error) {
			return "test-node", pods, nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "check success for node test-node")
		assert.Equal(t, "test-node", cr.NodeName)
		assert.Len(t, cr.Pods, 2)
	})
}

// TestCheckResult_String_WithContainerStates tests String method with various container states
func TestCheckResult_String_WithContainerStates(t *testing.T) {
	mockey.PatchConvey("checkResult String with container states", t, func() {
		now := metav1.Now()

		cr := &checkResult{
			NodeName: "test-node",
			Pods: []PodStatus{
				{
					Name:      "running-pod",
					Namespace: "default",
					ContainerStatuses: []ContainerStatus{
						{
							Name: "running-container",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: now,
								},
							},
						},
					},
				},
				{
					Name:      "terminated-pod",
					Namespace: "default",
					ContainerStatuses: []ContainerStatus{
						{
							Name: "terminated-container",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 0,
								},
							},
						},
					},
				},
				{
					Name:      "waiting-pod",
					Namespace: "default",
					ContainerStatuses: []ContainerStatus{
						{
							Name: "waiting-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "ContainerCreating",
								},
							},
						},
					},
				},
				{
					Name:      "unknown-pod",
					Namespace: "default",
					ContainerStatuses: []ContainerStatus{
						{
							Name:  "unknown-container",
							State: corev1.ContainerState{}, // All nil
						},
					},
				},
			},
		}

		result := cr.String()
		assert.Contains(t, result, "running")
		assert.Contains(t, result, "terminated")
		assert.Contains(t, result, "waiting")
		assert.Contains(t, result, "unknown")
	})
}

// TestCheckResult_String_EmptyPods tests String method with empty pods
func TestCheckResult_String_EmptyPods(t *testing.T) {
	mockey.PatchConvey("checkResult String with empty pods", t, func() {
		cr := &checkResult{
			NodeName: "test-node",
			Pods:     []PodStatus{},
		}

		result := cr.String()
		assert.Equal(t, "no pod found", result)
	})
}

// TestCheckResult_String_Nil tests String method with nil checkResult
func TestCheckResult_String_Nil(t *testing.T) {
	mockey.PatchConvey("nil checkResult String", t, func() {
		var cr *checkResult
		result := cr.String()
		assert.Equal(t, "", result)
	})
}

// TestCheckResult_Summary_Nil tests Summary method with nil checkResult
func TestCheckResult_Summary_Nil(t *testing.T) {
	mockey.PatchConvey("nil checkResult Summary", t, func() {
		var cr *checkResult
		result := cr.Summary()
		assert.Equal(t, "", result)
	})
}

// TestCheckResult_HealthStateType_Nil tests HealthStateType method with nil checkResult
func TestCheckResult_HealthStateType_Nil(t *testing.T) {
	mockey.PatchConvey("nil checkResult HealthStateType", t, func() {
		var cr *checkResult
		result := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateType(""), result)
	})
}

// TestCheckResult_getError_NilError tests getError method with nil error
func TestCheckResult_getError_NilError(t *testing.T) {
	mockey.PatchConvey("checkResult getError with nil error", t, func() {
		cr := &checkResult{
			err: nil,
		}
		result := cr.getError()
		assert.Equal(t, "", result)
	})
}

// TestCheckResult_getError_WithError tests getError method with error
func TestCheckResult_getError_WithError(t *testing.T) {
	mockey.PatchConvey("checkResult getError with error", t, func() {
		cr := &checkResult{
			err: errors.New("test error"),
		}
		result := cr.getError()
		assert.Equal(t, "test error", result)
	})
}

// TestCheckResult_getError_Nil tests getError method with nil checkResult
func TestCheckResult_getError_Nil(t *testing.T) {
	mockey.PatchConvey("nil checkResult getError", t, func() {
		var cr *checkResult
		result := cr.getError()
		assert.Equal(t, "", result)
	})
}

// TestCheckResult_HealthStates_NilResult tests HealthStates with nil checkResult
func TestCheckResult_HealthStates_NilResult(t *testing.T) {
	mockey.PatchConvey("nil checkResult HealthStates", t, func() {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})
}

// TestCheckResult_HealthStates_WithPods tests HealthStates with pods
func TestCheckResult_HealthStates_WithPods(t *testing.T) {
	mockey.PatchConvey("checkResult HealthStates with pods", t, func() {
		cr := &checkResult{
			ts:       time.Now().UTC(),
			health:   apiv1.HealthStateTypeHealthy,
			reason:   "check success",
			NodeName: "test-node",
			Pods: []PodStatus{
				{Name: "pod1", Namespace: "default"},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.NotNil(t, states[0].ExtraInfo)
		assert.Contains(t, states[0].ExtraInfo, "data")
	})
}

// TestCheckResult_HealthStates_NoPods tests HealthStates with no pods
func TestCheckResult_HealthStates_NoPods(t *testing.T) {
	mockey.PatchConvey("checkResult HealthStates with no pods", t, func() {
		cr := &checkResult{
			ts:       time.Now().UTC(),
			health:   apiv1.HealthStateTypeHealthy,
			reason:   "kubelet not running",
			NodeName: "",
			Pods:     nil,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Nil(t, states[0].ExtraInfo)
	})
}

// TestCheckResult_HealthStates_WithError tests HealthStates with error
func TestCheckResult_HealthStates_WithError(t *testing.T) {
	mockey.PatchConvey("checkResult HealthStates with error", t, func() {
		cr := &checkResult{
			ts:     time.Now().UTC(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "failed to list pods",
			err:    errors.New("connection refused"),
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "connection refused", states[0].Error)
	})
}

// TestComponent_LastHealthStates_NilLastCheckResult tests LastHealthStates with nil lastCheckResult
func TestComponent_LastHealthStates_NilLastCheckResult(t *testing.T) {
	mockey.PatchConvey("LastHealthStates with nil lastCheckResult", t, func() {
		c := &component{
			lastCheckResult: nil,
		}

		states := c.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})
}

// TestComponent_Events tests the Events method
func TestComponent_Events(t *testing.T) {
	mockey.PatchConvey("Events returns nil", t, func() {
		c := &component{}
		events, err := c.Events(context.Background(), time.Now())
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}

// TestComponent_IsSupported tests the IsSupported method
func TestComponent_IsSupported(t *testing.T) {
	mockey.PatchConvey("IsSupported returns true", t, func() {
		c := &component{}
		assert.True(t, c.IsSupported())
	})
}

// TestComponent_Close tests the Close method
func TestComponent_Close(t *testing.T) {
	mockey.PatchConvey("Close cancels context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		c := &component{
			ctx:    ctx,
			cancel: cancel,
		}

		err := c.Close()
		assert.NoError(t, err)

		// Verify context is canceled
		select {
		case <-ctx.Done():
			// Expected
		default:
			t.Error("Expected context to be canceled")
		}
	})
}

// TestComponent_Start tests the Start method
func TestComponent_Start(t *testing.T) {
	mockey.PatchConvey("Start begins background checking", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return false },
			checkKubeletRunning:   func() bool { return false },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}

		err := c.Start()
		assert.NoError(t, err)

		// Give it time to run at least once
		time.Sleep(100 * time.Millisecond)
	})
}

// TestComponent_Name tests the Name method
func TestComponent_Name(t *testing.T) {
	mockey.PatchConvey("Name returns correct name", t, func() {
		c := &component{}
		assert.Equal(t, Name, c.Name())
		assert.Equal(t, "kubelet", c.Name())
	})
}

// TestComponent_Tags tests the Tags method
func TestComponent_Tags(t *testing.T) {
	mockey.PatchConvey("Tags returns correct tags", t, func() {
		c := &component{}
		tags := c.Tags()
		assert.Contains(t, tags, "container")
		assert.Contains(t, tags, "kubelet")
		assert.Len(t, tags, 2)
	})
}

// TestCheckResult_ComponentName tests ComponentName method
func TestCheckResult_ComponentName(t *testing.T) {
	mockey.PatchConvey("ComponentName returns correct name", t, func() {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
		assert.Equal(t, "kubelet", cr.ComponentName())
	})
}

// TestComponent_Check_ContextCancellation tests Check with context cancellation
func TestComponent_Check_ContextCancellation(t *testing.T) {
	mockey.PatchConvey("Check with context cancellation", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}

		// The Check should still work but ListPodsFromKubeletReadOnlyPort will fail
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should have an error due to canceled context
		assert.NotNil(t, cr.err)
	})
}

// TestComponent_ConcurrentAccess tests concurrent access to component
func TestComponent_ConcurrentAccess(t *testing.T) {
	mockey.PatchConvey("concurrent access to component", t, func() {
		mockey.Mock(ListPodsFromKubeletReadOnlyPort).To(func(ctx context.Context, port int) (string, []PodStatus, error) {
			return "test-node", []PodStatus{{Name: "pod1"}}, nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := &component{
			ctx:                   ctx,
			cancel:                cancel,
			checkKubeletInstalled: func() bool { return true },
			checkKubeletRunning:   func() bool { return true },
			kubeletReadOnlyPort:   DefaultKubeletReadOnlyPort,
			failedCountThreshold:  defaultFailedCountThreshold,
		}

		// Run concurrent operations
		done := make(chan bool, 20)
		for i := 0; i < 10; i++ {
			go func() {
				c.Check()
				done <- true
			}()
			go func() {
				_ = c.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 20; i++ {
			<-done
		}

		// Verify final state is consistent
		states := c.LastHealthStates()
		assert.NotNil(t, states)
	})
}
