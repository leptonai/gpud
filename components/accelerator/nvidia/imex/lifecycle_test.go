package imex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config/common"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
)

type rackInstance struct{ nvidianvml.Instance }

func (*rackInstance) ProductName() string { return "NVIDIA GB300" }

func TestConstructorAndLifecycle(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "required"}[required], func(t *testing.T) {
			instance := &components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: &rackInstance{}}
			instance.NVIDIAToolOverwrites.IMEX.Required = &required
			cmp, err := New(instance)
			require.NoError(t, err)
			c := cmp.(*component)
			defer func() { require.NoError(t, c.Close()) }()
			require.Equal(t, Name, c.Name())
			require.Contains(t, c.Tags(), "nvidia")
			require.Equal(t, required, c.IsSupported())
			require.Equal(t, []string{"nvidia-imex-ctl"}, c.config.Command)
			require.Equal(t, []string{"systemctl", "is-active", "nvidia-imex"}, c.config.ServiceCommand)
			require.Equal(t, "/etc/nvidia-imex/config.cfg", c.config.ConfigFile)
			require.Equal(t, apiv1.NotEvaluatedNoData, c.LastHealthStates()[0].NotEvaluatedReason)
			events, err := c.Events(context.Background(), time.Now())
			require.NoError(t, err)
			require.Empty(t, events)
			c.run = func(ctx context.Context, args []string) (string, error) {
				require.NotEmpty(t, args)
				require.NoError(t, ctx.Err())
				if args[0] == "systemctl" {
					return "active", nil
				}
				require.Equal(t, []string{"nvidia-imex-ctl", "-c", "/etc/nvidia-imex/config.cfg", "-N"}, args)
				return healthySnapshot, nil
			}
			require.NoError(t, c.Start())
			require.Eventually(t, func() bool { return c.LastHealthStates()[0].Reason != "no data yet" }, time.Second, time.Millisecond)
			state := c.LastHealthStates()[0]
			if required {
				require.True(t, *state.Evaluated)
				require.Contains(t, state.ExtraInfo, "imex")
			} else {
				require.Equal(t, apiv1.NotEvaluatedUnsupported, state.NotEvaluatedReason)
			}
			require.NoError(t, c.Close())
			require.ErrorIs(t, c.ctx.Err(), context.Canceled)
		})
	}
	cmp, err := New(&components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: &rackInstance{}, NVIDIAToolOverwrites: common.ToolOverwrites{IMEX: common.IMEXConfig{Command: []string{"ctl"}, ServiceCommand: []string{"service"}, ConfigFile: "custom.cfg"}}})
	require.NoError(t, err)
	defer func(c components.Component) { require.NoError(t, c.Close()) }(cmp)
	require.True(t, cmp.IsSupported())
	c := cmp.(*component)
	require.Equal(t, "custom.cfg", c.config.ConfigFile)
	cmp, err = New(&components.GPUdInstance{RootCtx: context.Background()})
	require.NoError(t, err)
	defer func(c components.Component) { require.NoError(t, c.Close()) }(cmp)
	require.False(t, cmp.IsSupported())
}

func TestDomainEvidenceFailures(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement, reason string
		expected                       []string
		partial                        bool
	}{
		{name: "local daemon", old: "* - READY", replacement: "* - STARTING", reason: "local IMEX daemon reports STARTING"},
		{name: "peer daemon", old: "192.0.2.2 - READY", replacement: "192.0.2.2 - STARTING", reason: "peer 1 (192.0.2.2) daemon reports STARTING"},
		{name: "remote observation", old: "1 C C", replacement: "1 D C", reason: "observer 1 to peer 0: disconnected"},
		{name: "domain", old: "Domain State: UP", replacement: "Domain State: DOWN", reason: "IMEX domain reports DOWN"},
		{name: "partial remote", old: "1 C C", replacement: "1 I C", reason: "observer 1 to peer 0: unavailable", partial: true},
		{name: "inventory size", reason: "membership count differs", expected: []string{"192.0.2.1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := healthySnapshot
			if tc.old != "" {
				output = strings.Replace(output, tc.old, tc.replacement, 1)
			}
			c := testComponent(output, nil)
			c.config.ExpectedAddresses = tc.expected
			r := c.Check()
			require.Equal(t, Name, r.ComponentName())
			require.Equal(t, r.Summary(), r.String())
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, r.HealthStateType())
			state := r.HealthStates()[0]
			require.Contains(t, state.Reason, tc.reason)
			require.Equal(t, apiv1.FaultScopeIMEXDomain, state.FaultScope)
			require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, state.SuggestedActions.RepairActions)
			require.Equal(t, !tc.partial, *state.Evaluated)
			if tc.partial {
				require.Equal(t, apiv1.NotEvaluatedPartial, state.NotEvaluatedReason)
			}
			var snapshot Snapshot
			require.NoError(t, json.Unmarshal([]byte(state.ExtraInfo["imex"]), &snapshot))
			require.Equal(t, 0, snapshot.NodeID)
		})
	}
	c := testComponent("malformed", nil)
	state := c.Check().HealthStates()[0]
	require.Equal(t, apiv1.NotEvaluatedCollection, state.NotEvaluatedReason)
	require.Contains(t, state.Error, "incomplete IMEX snapshot")
	c = testComponent(healthySnapshot, nil)
	c.config.ExpectedAddresses = []string{"192.0.2.1", "192.0.2.2"}
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
	c.run = func(_ context.Context, args []string) (string, error) {
		if args[0] == "service" {
			return "failed", errors.New("exit 3")
		}
		return healthySnapshot, nil
	}
	require.Contains(t, c.Check().Summary(), "local IMEX daemon is failed")
	var noData *result
	require.Equal(t, "no data yet", noData.Summary())
	require.Equal(t, apiv1.HealthStateTypeHealthy, noData.HealthStateType())
}

func TestCommandCaptureIsBoundedAndCancelable(t *testing.T) {
	output, err := runCommand(context.Background(), []string{"sh", "-c", "printf stdout; printf stderr >&2; exit 3"})
	require.Error(t, err)
	require.Contains(t, output, "stdout")
	require.Contains(t, output, "stderr")
	_, err = runCommand(context.Background(), []string{"sh", "-c", "head -c 2097153 /dev/zero"})
	require.ErrorContains(t, err, "IMEX output exceeds")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = runCommand(ctx, []string{"sh", "-c", "exec sleep 5"})
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestMalformedSnapshotIdentityAndRows(t *testing.T) {
	for _, tc := range []struct{ old, replacement, reason string }{
		{"#1 -", "#9999999999999999999999999999 -", "value out of range"},
		{`Nodes From\To 0 1`, `Nodes From\To 0 x`, "invalid matrix column"},
		{"1 C C", "1x C C", "invalid syntax"},
		{"1 C C", "0 C C", "duplicate matrix row"},
		{"Node #0 *", "Node #0 -", "incomplete IMEX snapshot"},
		{`Nodes From\To 0 1`, `Nodes From\To 0 2`, "missing node or matrix row"},
	} {
		_, err := ParseSnapshot(strings.Replace(healthySnapshot, tc.old, tc.replacement, 1))
		require.ErrorContains(t, err, tc.reason)
	}
	_, err := ParseSnapshot(strings.Replace(healthySnapshot, `Nodes From\To 0 1`, `Nodes From\To 0 1`+"\n"+`Nodes From\To 0 1`, 1))
	require.ErrorContains(t, err, "multiple connectivity matrices")
}
