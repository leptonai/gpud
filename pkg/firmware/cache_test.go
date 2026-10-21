package firmware

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func resetCache(t *testing.T) {
	t.Helper()
	latest.Store(nil)
	t.Cleanup(func() { latest.Store(nil) })
}

// gpuOnlyOptions collects only the fake GPUs: a non-Linux GOOS skips the
// sysfs and dpkg sources.
func gpuOnlyOptions(now func() time.Time, gpus ...fakeGPU) Options {
	return Options{NVMLInstance: newFakeNVML(gpus...), goos: "darwin", now: now}
}

func TestLatest_BeforeAndAfterRefresh(t *testing.T) {
	resetCache(t)
	assert.Nil(t, Latest())

	Refresh(t.Context(), gpuOnlyOptions(func() time.Time { return testReadAt },
		fakeGPU{busID: "0000:02:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:01:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
	))
	want := []apiv1.MachineFirmware{row(apiv1.FirmwareComponentGPU, "97.10.4A.00.0B", "gpu1", "gpu2")}
	assert.Equal(t, want, Latest())

	// Callers own the returned copy.
	got := Latest()
	got[0].Version = "changed"
	got[0].InstanceNames[0] = "changed"
	assert.Equal(t, want, Latest())
}

func TestRefresh_NothingFoundClearsCache(t *testing.T) {
	resetCache(t)
	Refresh(t.Context(), gpuOnlyOptions(nil, fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS}))
	require.NotNil(t, Latest())

	Refresh(t.Context(), gpuOnlyOptions(nil))
	assert.Nil(t, Latest())
}

func TestRefresh_CanceledKeepsCache(t *testing.T) {
	resetCache(t)
	opts := gpuOnlyOptions(func() time.Time { return testReadAt }, fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS})
	Refresh(t.Context(), opts)
	want := Latest()
	require.NotNil(t, want)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	Refresh(ctx, gpuOnlyOptions(nil))
	assert.Equal(t, want, Latest())

	// A missed deadline keeps the cache too.
	deadlineCtx, deadlineCancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	Refresh(deadlineCtx, gpuOnlyOptions(nil))
	assert.Equal(t, want, Latest())
}

func TestRefresh_FailedGPUReadDropsOnlyThatGPU(t *testing.T) {
	resetCache(t)
	now := func() time.Time { return testReadAt }
	Refresh(t.Context(), gpuOnlyOptions(now,
		fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:02:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS},
	))
	require.Equal(t, []apiv1.MachineFirmware{row(apiv1.FirmwareComponentGPU, "96.00.00.00.01", "gpu1", "gpu2")}, Latest())

	// The failed GPU is absent until a later read succeeds; the other keeps its name.
	Refresh(t.Context(), gpuOnlyOptions(now,
		fakeGPU{busID: "0000:01:00.0", ret: nvml.ERROR_GPU_IS_LOST},
		fakeGPU{busID: "0000:02:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS},
	))
	assert.Equal(t, []apiv1.MachineFirmware{row(apiv1.FirmwareComponentGPU, "96.00.00.00.01", "gpu2")}, Latest())
}

func TestRefresh_ConcurrentWithLatest(t *testing.T) {
	resetCache(t)
	opts := gpuOnlyOptions(nil,
		fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:02:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS},
	)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				Refresh(t.Context(), opts)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if rows := Latest(); rows != nil {
					rows[0].InstanceNames[0] = "mutated"
				}
			}
		}()
	}
	wg.Wait()

	rows := Latest()
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"gpu1", "gpu2"}, rows[0].InstanceNames)
}

func TestRunPeriodic(t *testing.T) {
	resetCache(t)

	// A non-positive interval disables the loop.
	RunPeriodic(t.Context(), gpuOnlyOptions(nil), 0)

	var reads atomic.Int64
	now := func() time.Time {
		return testReadAt.Add(time.Duration(reads.Add(1)) * time.Hour)
	}
	opts := gpuOnlyOptions(now, fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunPeriodic(ctx, opts, time.Millisecond)
	}()

	require.Eventually(t, func() bool { return reads.Load() >= 3 }, 5*time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunPeriodic did not return after cancel")
	}

	rows := Latest()
	require.Len(t, rows, 1)
	assert.True(t, rows[0].LastUpdated.After(testReadAt))
}

func TestStartAndWaitFirst(t *testing.T) {
	resetCache(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := Start(ctx, gpuOnlyOptions(nil, fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS}), DefaultCollectInterval)

	require.True(t, WaitFirst(t.Context(), ready, 5*time.Second))
	rows := Latest()
	require.Len(t, rows, 1)
	assert.Equal(t, "96.00.00.00.01", rows[0].Version)

	// Already ready wins over a done context.
	doneCtx, doneCancel := context.WithCancel(t.Context())
	doneCancel()
	assert.True(t, WaitFirst(doneCtx, ready, time.Millisecond))
}

func TestWaitFirst_NotReady(t *testing.T) {
	never := make(chan struct{})
	assert.False(t, WaitFirst(t.Context(), never, time.Millisecond))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.False(t, WaitFirst(ctx, never, time.Hour))
}
