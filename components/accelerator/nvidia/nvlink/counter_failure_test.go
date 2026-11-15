package nvlink

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestNVLinkCounterFailureIsNotHealthyTelemetry(t *testing.T) {
	for _, counter := range []nvml.NvLinkErrorCounter{nvml.NVLINK_ERROR_DL_REPLAY, nvml.NVLINK_ERROR_DL_RECOVERY, nvml.NVLINK_ERROR_DL_CRC_FLIT} {
		for _, ret := range []nvml.Return{nvml.ERROR_UNKNOWN, nvml.ERROR_GPU_IS_LOST, nvml.ERROR_RESET_REQUIRED, nvml.ERROR_NOT_SUPPORTED} {
			t.Run(fmt.Sprintf("counter=%d/return=%v", counter, ret), func(t *testing.T) {
				dev := testutil.NewMockDevice(&mock.Device{
					GetNvLinkStateFunc: func(int) (nvml.EnableState, nvml.Return) { return nvml.FEATURE_ENABLED, nvml.SUCCESS },
					GetNvLinkErrorCounterFunc: func(link int, c nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
						// The last link has no subsequent state query to expose a
						// failure that starts while reading its error counters.
						if link == 35 && c == counter {
							return 0, ret
						}
						return 7, nvml.SUCCESS
					},
				}, "rubin", "NVIDIA", "", "0000:01:00.0")
				links, err := getRubinNVLink("GPU-rubin", dev)
				if ret == nvml.ERROR_NOT_SUPPORTED {
					require.NoError(t, err, "optional counters can be unsupported")
				} else {
					require.Error(t, err)
					switch ret {
					case nvml.ERROR_GPU_IS_LOST:
						require.ErrorIs(t, err, nvmlerrors.ErrGPULost)
					case nvml.ERROR_RESET_REQUIRED:
						require.ErrorIs(t, err, nvmlerrors.ErrGPURequiresReset)
					}
					require.Contains(t, err.Error(), "link 35")
				}
				require.Len(t, links.States, 36, "retain the link state read before its counter failed")
			})
		}
	}
}
