package device

import (
	"bytes"
	"encoding/json"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

type fabricV4Device struct {
	nvlibdevice.Device
	info nvml.GpuFabricInfo_v4
	ret  nvml.Return
}

func (d *fabricV4Device) GetFabricInfoV4() (nvml.GpuFabricInfo_v4, nvml.Return) { return d.info, d.ret }

func TestFabricV4TypedCliques(t *testing.T) {
	info := nvml.GpuFabricInfo_v4{NumCliques: 6, State: nvml.GPU_FABRIC_STATE_COMPLETED,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
		HealthMask:    uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY}
	info.ClusterUuid[0] = 1
	// Exercise sorting, repeated types and an unknown type without discarding data.
	copy(info.Cliques[:], []nvml.GpuFabricClique_v1{{Type: 3, Id: 32766}, {Type: 0, Id: 65534},
		{Type: 2, Id: 32766}, {Type: 1, Id: 65534}, {Type: 0, Id: 65535}, {Type: 9, Id: 42}})
	state, err := (&nvDevice{rubin: true, Device: &fabricV4Device{info: info}}).GetFabricState()
	require.NoError(t, err)
	require.Equal(t, uint32(65534), *state.LegacyCliqueID())
	require.Equal(t, []apiv1.GPUFabricClique{{Type: 0, ID: 65534}, {Type: 0, ID: 65535}, {Type: 1, ID: 65534},
		{Type: 2, ID: 32766}, {Type: 3, ID: 32766}, {Type: 9, ID: 42}}, state.Cliques)
	require.Equal(t, "01000000-0000-0000-0000-000000000000", state.ClusterUUID)
	require.Contains(t, state.GetIssues(), "summary=Unhealthy")
	require.Contains(t, state.GetIssues(), "route unhealthy")
	entry := state.ToEntry("GPU-rubin")
	var out bytes.Buffer
	entry.RenderTable(&out)
	for _, label := range []string{"Unicast Pointer", "Multicast Pointer", "Unicast LE", "Multicast LE", "Unknown Type 9", "65534", "32766"} {
		require.Contains(t, out.String(), label)
	}
	payload, err := json.Marshal(entry)
	require.NoError(t, err)
	var decoded FabricStateEntry
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, entry, decoded)
}

func TestFabricV4MissingAndZeroLegacyClique(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cliques []nvml.GpuFabricClique_v1
		want    *uint32
	}{
		{name: "no assignments"},
		{name: "logical endpoint only", cliques: []nvml.GpuFabricClique_v1{{Type: 2, Id: 32766}}},
		{name: "valid zero", cliques: []nvml.GpuFabricClique_v1{{Type: 0, Id: 0}}, want: new(uint32)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := nvml.GpuFabricInfo_v4{NumCliques: uint32(len(tc.cliques))}
			copy(info.Cliques[:], tc.cliques)
			state, err := (&nvDevice{rubin: true, Device: &fabricV4Device{info: info}}).GetFabricState()
			require.NoError(t, err)
			require.NotNil(t, state.Cliques)
			require.Equal(t, tc.want, state.LegacyCliqueID())
			payload, err := json.Marshal(state.ToEntry("GPU"))
			require.NoError(t, err)
			if tc.want == nil {
				require.NotContains(t, string(payload), "clique_id")
			} else {
				require.Contains(t, string(payload), `"clique_id":0`)
			}
		})
	}
	_, err := (&nvDevice{rubin: true, Device: &fabricV4Device{info: nvml.GpuFabricInfo_v4{NumCliques: 65}}}).GetFabricState()
	require.ErrorContains(t, err, "invalid clique count")
}

func TestFabricV4FallbackOnlyForAPICompatibility(t *testing.T) {
	for _, tc := range []struct {
		ret     nvml.Return
		wantErr error
	}{
		{nvml.ERROR_GPU_IS_LOST, nvmlerrors.ErrGPULost}, {nvml.ERROR_RESET_REQUIRED, nvmlerrors.ErrGPURequiresReset},
		{nvml.ERROR_UNKNOWN, nvml.ERROR_UNKNOWN}, {nvml.ERROR_NO_PERMISSION, nvml.ERROR_NO_PERMISSION},
		{nvml.ERROR_NOT_SUPPORTED, nil}, {nvml.ERROR_FUNCTION_NOT_FOUND, nil}, {nvml.ERROR_ARGUMENT_VERSION_MISMATCH, nil},
	} {
		t.Run(tc.ret.String(), func(t *testing.T) {
			mockey.PatchConvey("V4 health errors must not be hidden by legacy queries", t, func() {
				info := nvml.GpuFabricInfo_v3{CliqueId: 7, State: nvml.GPU_FABRIC_STATE_COMPLETED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}
				mockey.Mock(nvml.GpuFabricInfoHandler.V3).Return(info, nvml.SUCCESS).Build()
				fallbackCalled := false
				dev := &nvDevice{rubin: true, driverMajor: 550, Device: &fabricV4Device{ret: tc.ret, Device: newStubDevice(&mock.Device{
					GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { fallbackCalled = true; return nvml.GpuFabricInfoHandler{} },
				})}}
				state, err := dev.GetFabricState()
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					require.False(t, fallbackCalled)
				} else {
					require.NoError(t, err)
					require.True(t, fallbackCalled)
					require.Nil(t, state.Cliques)
					require.Equal(t, uint32(7), *state.LegacyCliqueID())
					require.Contains(t, state.GetIssues(), "summary=Unhealthy")
				}
			})
		})
	}
}

func TestLegacyFabricFaultBitsRemainInformational(t *testing.T) {
	mask := uint32(nvml.GPU_FABRIC_HEALTH_MASK_GFM_STATE_DISCONNECTED)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_GFM_STATE |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NO_PARTITION)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_INCORRECT_CONFIGURATION
	state := FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY, HealthMask: mask}
	require.Empty(t, state.GetIssues())
	require.Equal(t, "Disconnected", state.ToEntry("GPU").Health.GFMState)
	state.Rubin = true
	t.Setenv(product.EnvEnableRubinHealthRules, "")
	require.Empty(t, state.GetIssues(), "unqualified configuration flags remain telemetry only")
	require.Equal(t, "No Partition", state.ToEntry("GPU").Health.IncorrectConfiguration)
	t.Setenv(product.EnvEnableRubinHealthRules, "true")
	require.Contains(t, state.GetIssues(), "global fabric manager disconnected")
	require.Contains(t, state.GetIssues(), "incorrect configuration=No Partition")
}

func TestLegacyV3UnknownErrorStillFallsBack(t *testing.T) {
	mockey.PatchConvey("the binding upgrade preserves legacy fallback", t, func() {
		mockey.Mock(nvml.GpuFabricInfoHandler.V3).Return(nvml.GpuFabricInfo_v3{}, nvml.ERROR_UNKNOWN).Build()
		dev := &nvDevice{driverMajor: 550, Device: newStubDevice(&mock.Device{
			GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} },
			GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) {
				return nvml.GpuFabricInfo{State: nvml.GPU_FABRIC_STATE_COMPLETED, CliqueId: 7}, nvml.SUCCESS
			},
		})}
		state, err := dev.GetFabricState()
		require.NoError(t, err)
		require.Equal(t, uint32(7), *state.LegacyCliqueID())
	})
}
