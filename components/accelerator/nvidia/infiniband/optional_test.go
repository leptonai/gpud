package infiniband

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
)

func TestInfiniBandCollectorSelection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), ExperimentalHealthValidation: enabled, NVIDIAToolOverwrites: configcommon.ToolOverwrites{InfinibandClassRootDir: testClassDir}})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, comp.Close()) })
			devices, err := comp.(*component).getClassDevicesFunc(t.Context())
			require.NoError(t, err)
			ethernet := 0
			ib := 0
			for _, device := range devices {
				for _, port := range device.Ports {
					if port.LinkLayer == "Ethernet" {
						ethernet++
					}
					if port.LinkLayer == "InfiniBand" {
						ib++
					}
				}
			}
			require.Positive(t, ib)
			if enabled {
				require.Zero(t, ethernet)
			} else {
				require.Positive(t, ethernet)
			}
		})
	}
}
