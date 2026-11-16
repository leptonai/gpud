package device

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

func TestFabricV3FallbackOnlyForAPICompatibility(t *testing.T) {
	for _, tc := range []struct {
		ret     nvml.Return
		wantErr error
	}{
		{nvml.ERROR_GPU_IS_LOST, nvmlerrors.ErrGPULost},
		{nvml.ERROR_RESET_REQUIRED, nvmlerrors.ErrGPURequiresReset},
		{nvml.ERROR_UNKNOWN, nvml.ERROR_UNKNOWN},
		{nvml.ERROR_NO_PERMISSION, nvml.ERROR_NO_PERMISSION},
		{nvml.ERROR_NOT_SUPPORTED, nil},
		{nvml.ERROR_FUNCTION_NOT_FOUND, nil},
		{nvml.ERROR_ARGUMENT_VERSION_MISMATCH, nil},
	} {
		t.Run(tc.ret.String(), func(t *testing.T) {
			mockey.PatchConvey("real V3 failures cannot discard fabric health fields", t, func() {
				mockey.Mock(nvml.GpuFabricInfoHandler.V3).Return(nvml.GpuFabricInfo_v3{}, tc.ret).Build()
				v1Called := false
				dev := &nvDevice{rubin: true,
					driverMajor: MinDriverVersionForV3FabricAPI,
					Device: newStubDevice(&mock.Device{
						GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} },
						GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) {
							v1Called = true
							return nvml.GpuFabricInfo{State: nvml.GPU_FABRIC_STATE_COMPLETED}, nvml.SUCCESS
						},
					}),
				}
				state, err := dev.GetFabricState()
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					require.False(t, v1Called)
				} else {
					require.NoError(t, err)
					require.True(t, v1Called)
					require.Equal(t, uint8(nvml.GPU_FABRIC_STATE_COMPLETED), state.State)
				}
			})
		})
	}
}
