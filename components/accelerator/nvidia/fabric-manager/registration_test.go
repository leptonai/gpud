package fabricmanager

import (
	"context"
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
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType())

	report.Failed = true
	report.Reason = "GPU GPU-1: summary=Unhealthy"
	cr := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "summary=Unhealthy")
}
