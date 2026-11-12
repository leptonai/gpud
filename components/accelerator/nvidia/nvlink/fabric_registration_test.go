package nvlink

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

type fabricTestDevice struct {
	device.Device
	state device.FabricState
	err   error
}

func (d *fabricTestDevice) GetFabricState() (device.FabricState, error) { return d.state, d.err }

type fabricTestInstance struct{ mockNVMLInstance }
type namedFabricTestInstance struct {
	fabricTestInstance
	name string
}

func (n *namedFabricTestInstance) ProductName() string { return n.name }

func (*fabricTestInstance) FabricStateSupported() bool { return true }

func TestCheckFabricRegistrationBeforePeerFailure(t *testing.T) {
	start := time.Date(2026, 11, 5, 7, 33, 14, 0, time.UTC)
	now := start
	dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}}
	devs := map[string]device.Device{}
	for _, uuid := range []string{"GPU-0", "GPU-1", "GPU-2", "GPU-3", "GPU-4", "GPU-5", "GPU-6", "GPU-7"} {
		devs[uuid] = dev
	}
	c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
		return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: true}}}, nil
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}
	c.getTimeNowFunc = func() time.Time { return now }
	peer := p2pStatusNotSupported
	c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return peer, nil }
	c.getThresholdsFunc = func() ExpectedLinkStates { return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 8} }

	cr := c.Check().(*checkResult)
	require.Equal(t, 28, cr.PeerNVLinkProbePairCount)
	require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
	require.Contains(t, cr.reason, "registration in progress")
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.LastHealthStates()[0].Health)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.HealthStates()[0].SuggestedActions.RepairActions)

	// Replay the reported 77-second recovery: completed registration and P2P OK.
	now = start.Add(77 * time.Second)
	dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
	peer = p2pStatusOK
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())

	// Completed registration does not suppress a real all-NS topology failure.
	peer = p2pStatusNotSupported
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType())

	// A new incomplete episode has a deadline even if the state alternates.
	now = now.Add(time.Hour)
	dev.state.State = nvml.GPU_FABRIC_STATE_NOT_STARTED
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
	now = now.Add(fabric.RegistrationTimeout)
	dev.state.State = nvml.GPU_FABRIC_STATE_IN_PROGRESS
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "did not complete")
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)

	// An explicit fabric fault is immediate, including on the first observation.
	now = now.Add(time.Second)
	dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
	peer = p2pStatusOK
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
	now = now.Add(time.Second)
	dev.state.State = nvml.GPU_FABRIC_STATE_IN_PROGRESS
	dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "health check failed")
	require.Contains(t, cr.reason, "summary=Unhealthy")

	// GPU loss still takes precedence over a registration grace period.
	dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED
	c.getNVLinkFunc = func(string, device.Device) (NVLink, error) { return NVLink{}, nvmlerrors.ErrGPULost }
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
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

	dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS}}
	devs := map[string]device.Device{"GPU-0": dev, "GPU-1": dev}
	c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
		blockFirstCollection()
		return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: true}}}, nil
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}
	c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusOK, nil }

	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	var clocks atomic.Int32
	c.getTimeNowFunc = func() time.Time { return start.Add(time.Duration((clocks.Add(1)-1)/2) * time.Second) }
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

func TestCheckUnsupportedFabricKeepsPeerAndPortChecks(t *testing.T) {
	for _, productName := range []string{"NVIDIA H100 NVL", "NVIDIA H200 NVL", "NVIDIA H100 80GB HBM3", "NVIDIA-GB200"} {
		for _, count := range []int{2, 4, 8} {
			for _, apiError := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/apiError=%v", productName, count, apiError), func(t *testing.T) {
					dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED, Status: nvml.ERROR_UNKNOWN}}
					if apiError {
						dev.err = nvml.ERROR_NOT_SUPPORTED
					}
					devs := map[string]device.Device{}
					for i := 0; i < count; i++ {
						devs[fmt.Sprint("GPU-", i)] = dev
					}
					active := true
					c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
						return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: active}}}, nil
					}).(*component)
					defer func() { require.NoError(t, c.Close()) }()
					c.nvmlInstance = &namedFabricTestInstance{fabricTestInstance: fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}, name: productName}
					c.checkNVSwitchExistsFunc = func() bool { return false }
					c.getThresholdsFunc = func() ExpectedLinkStates { return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: count} }
					peer := p2pStatusOK
					c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return peer, nil }
					require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
					peer = p2pStatusNotSupported
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType(), "unsupported fabric must not suppress P2P failures")
					peer = p2pStatusOK
					active = false
					cr := c.Check().(*checkResult)
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					wantAction := apiv1.RepairActionTypeRebootSystem
					if product.MayHaveExternalFabricByGPUProduct(productName) {
						wantAction = apiv1.RepairActionTypeIgnoreNoActionRequired
					}
					require.Equal(t, []apiv1.RepairActionType{wantAction}, cr.suggestedActions.RepairActions)
					active = true
					devs["GPU-0"] = &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}}
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType(), "mixed support must remain checked")
					devs["GPU-0"] = dev
					c.checkNVSwitchExistsFunc = func() bool { return true }
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType(), "detected switches require telemetry")
				})
			}
		}
	}
}

func TestCheckFabricFaultRetainsDetailsWithoutSpeculativeReboot(t *testing.T) {
	for _, state := range []device.FabricState{
		{State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.ERROR_UNKNOWN},
		{State: nvml.GPU_FABRIC_STATE_COMPLETED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY},
	} {
		dev := &fabricTestDevice{state: state}
		devs := map[string]device.Device{"GPU-0": dev, "GPU-1": dev}
		c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: false}}}, nil
		}).(*component)
		defer func() { require.NoError(t, c.Close()) }()
		c.nvmlInstance = &fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}
		c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusNotSupported, nil }
		cr := c.Check().(*checkResult)
		require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		require.Contains(t, cr.reason, "inactive nvlinks=GPU-0,GPU-1")
		require.Contains(t, cr.reason, "peer nvlink p2p statuses=NS")
		require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)
	}
}
