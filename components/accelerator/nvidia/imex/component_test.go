package imex

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/config/common"
)

const healthySnapshot = `Nodes:
Node #0 * 192.0.2.1 * - READY - Version: 580.00 - Hostname: tray-0
Node #1 - 192.0.2.2 - READY - Version: 580.00 - Hostname: tray-1
Nodes From\To 0 1
0 C C
1 C C
Domain State: UP
`

func TestSnapshotValidation(t *testing.T) {
	s, err := ParseSnapshot(healthySnapshot)
	require.NoError(t, err)
	require.Equal(t, 0, s.NodeID)
	require.Equal(t, "connected", s.Peers[0].Status)
	for _, output := range []string{
		strings.Replace(healthySnapshot, "1 C C\n", "", 1),
		strings.Replace(healthySnapshot, "0 C C", "0 C ?", 1),
		strings.Replace(healthySnapshot, "0 C C", "0 C", 1),
		strings.Replace(healthySnapshot, "Nodes From\\To 0 1", "Nodes From\\To 0 0", 1),
		strings.Replace(healthySnapshot, "Node #1 -", "Node #1 *", 1),
		strings.Replace(healthySnapshot, "Domain State: UP", "", 1),
		healthySnapshot + healthySnapshot,
	} {
		_, err := ParseSnapshot(output)
		require.Error(t, err, output)
	}
}

func TestPeerFailuresAndMissingEvidence(t *testing.T) {
	for code, status := range statuses {
		t.Run(code, func(t *testing.T) {
			output := strings.Replace(healthySnapshot, "0 C C", "0 C "+code, 1)
			c := testComponent(output, nil)
			s := c.Check().HealthStates()[0]
			if code == "C" {
				require.Equal(t, apiv1.HealthStateTypeHealthy, s.Health)
			} else {
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, s.Health)
				require.Contains(t, s.Reason, "192.0.2.2")
				require.Contains(t, s.Reason, status)
				require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, s.SuggestedActions.RepairActions)
			}
			require.Contains(t, s.ExtraInfo, "imex")
		})
	}
	c := testComponent("", errors.New("command service disabled"))
	s := c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeDegraded, s.Health)
	require.False(t, *s.Evaluated)
	require.Nil(t, s.SuggestedActions)
	c.run = func(_ context.Context, args []string) (string, error) {
		if args[0] == "service" {
			return "inactive\n", errors.New("exit 3")
		}
		return "", errors.New("unreachable")
	}
	s = c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, s.Health)
	require.Contains(t, s.Reason, "inactive")
	require.False(t, *s.Evaluated)
}

func TestRequiredAndInventory(t *testing.T) {
	c := testComponent(healthySnapshot, nil)
	c.required = false
	require.False(t, *c.Check().HealthStates()[0].Evaluated)
	c.required = true
	c.config.ExpectedAddresses = []string{"192.0.2.1", "192.0.2.3"}
	s := c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, s.Health)
	require.Contains(t, s.Reason, "192.0.2.3")
}

func testComponent(output string, err error) *component {
	return &component{ctx: context.Background(), required: true, config: common.IMEXConfig{Command: []string{"ctl"}, ServiceCommand: []string{"service"}}, run: func(_ context.Context, args []string) (string, error) {
		if args[0] == "service" {
			return "active", nil
		}
		return output, err
	}}
}
