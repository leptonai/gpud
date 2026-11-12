package fabricmanager

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"
)

func TestCheckFabricRegistrationWhileManagerStarts(t *testing.T) {
	start := time.Now().UTC()
	now := start
	report := fabricStateReport{Pending: true, Reason: "GPU GPU-0: state=In Progress"}
	active := false
	c := &component{
		ctx: context.Background(), cancel: func() {},
		nvmlInstance:            &mockNVMLInstance{exists: true, supportsFM: true, supportsFabricState: true, productName: "NVIDIA H200", deviceCount: 8},
		getTimeNowFunc:          func() time.Time { return now },
		collectFabricStateFunc:  func() fabricStateReport { return report },
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return active },
	}
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.LastHealthStates()[0].Health)
	now = start.Add(77 * time.Second)
	active = true
	report = fabricStateReport{Healthy: true}
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())

	now = start.Add(time.Hour)
	active = false
	report = fabricStateReport{Pending: true}
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
	now = now.Add(fabric.RegistrationTimeout)
	timedOut := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, timedOut.health)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, timedOut.suggestedActions.RepairActions)

	report.Failed = true
	report.Reason = "GPU GPU-1: summary=Unhealthy"
	cr := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "summary=Unhealthy")
}

// The first collection is slow while a second trigger tries to finish first.
// Distinct timestamps expose stale registration observations if checks overlap.
func TestCheckSerializesFabricObservations(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var collections atomic.Int32
	blockFirstCollection := func() {
		if collections.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	c := &component{
		ctx: context.Background(), cancel: func() {},
		nvmlInstance: &mockNVMLInstance{exists: true, supportsFM: true, supportsFabricState: true, productName: "NVIDIA H200", deviceCount: 8},
		collectFabricStateFunc: func() fabricStateReport {
			blockFirstCollection()
			return fabricStateReport{Pending: true}
		},
		checkNVSwitchExistsFunc: func() bool { return true },
		checkFMExistsFunc:       func() bool { return true },
		checkFMActiveFunc:       func() bool { return true },
	}

	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	var clocks atomic.Int32
	c.getTimeNowFunc = func() time.Time { return start.Add(time.Duration(clocks.Add(1)-1) * time.Second) }
	results := make(chan *checkResult, 2)
	go func() { results <- c.Check().(*checkResult) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first collection did not start")
	}
	secondStarted := make(chan struct{})
	go func() { close(secondStarted); results <- c.Check().(*checkResult) }()
	<-secondStarted
	select {
	case <-results:
		t.Fatal("second check overtook blocked collection")
	case <-time.After(100 * time.Millisecond):
	}
	require.EqualValues(t, 1, clocks.Load(), "timestamp must be captured after the preceding check completes")
	unblock()
	seen := map[time.Time]bool{}
	for range 2 {
		select {
		case cr := <-results:
			require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
			seen[cr.ts] = true
		case <-time.After(5 * time.Second):
			t.Fatal("check did not finish")
		}
	}
	require.True(t, seen[start])
	require.True(t, seen[start.Add(time.Second)])
	require.Equal(t, start.Add(time.Second), c.lastCheckResult.ts)
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.LastHealthStates()[0].Health)
}
