package nvlink

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestLegacyNVLinkRangeAndErrorTolerance(t *testing.T) {
	mockey.PatchConvey("legacy families retain 18 links and tolerant counters", t, func() {
		called := 0
		mockey.Mock(nvml.DeviceGetNvLinkState).To(func(_ nvml.Device, link int) (nvml.EnableState, nvml.Return) {
			called++
			require.Less(t, link, 18)
			if link == 2 {
				return nvml.FEATURE_DISABLED, nvml.ERROR_INVALID_ARGUMENT
			}
			if link == 1 {
				return nvml.FEATURE_DISABLED, nvml.ERROR_UNKNOWN
			}
			return nvml.FEATURE_ENABLED, nvml.SUCCESS
		}).Build()
		mockey.Mock(nvml.DeviceGetNvLinkErrorCounter).Return(uint64(0), nvml.ERROR_UNKNOWN).Build()
		dev := testutil.NewMockDevice(&mock.Device{}, "hopper", "NVIDIA", "9.0", "")
		result, err := GetNVLink("GPU-h100", dev)
		require.NoError(t, err)
		require.Equal(t, 18, called)
		require.Len(t, result.States, 16)
		require.Equal(t, 17, result.States[len(result.States)-1].Link)
	})
}
