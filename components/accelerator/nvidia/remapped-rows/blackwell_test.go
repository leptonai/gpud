package remappedrows

import (
	"context"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestBlackwellRemappedRowsRecovery(t *testing.T) {
	for _, name := range []string{"NVIDIA GB300", "NVIDIA-GB300", "NVIDIA B300 SXM6 AC", "NVIDIA-B300-SXM6-AC"} {
		for _, tc := range []struct {
			name   string
			rows   RemappedRows
			action apiv1.RepairActionType
		}{
			{name: "correctable remaps alone", rows: RemappedRows{RemappedDueToCorrectableErrors: 5}},
			{name: "applied uncorrectable remaps", rows: RemappedRows{RemappedDueToUncorrectableErrors: 2}},
			{name: "pending", rows: RemappedRows{RemappedDueToUncorrectableErrors: 2, RemappingPending: true}, action: apiv1.RepairActionTypeRebootSystem},
			{name: "failure", rows: RemappedRows{RemappingFailed: true}, action: apiv1.RepairActionTypeHardwareInspection},
			{name: "failure with pending", rows: RemappedRows{RemappingPending: true, RemappingFailed: true}, action: apiv1.RepairActionTypeHardwareInspection},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				dev := testutil.NewMockDevice(&mock.Device{}, "blackwell", "NVIDIA", "13.0", "0008:06:00.0")
				instance := &mockNVMLInstance{
					getDevicesFunc:     func() map[string]device.Device { return map[string]device.Device{"GPU-0": dev} },
					getProductNameFunc: func() string { return name },
					getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
						return product.SupportedMemoryMgmtCapsByGPUProduct(name)
					},
				}
				comp, err := New(&components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: instance})
				require.NoError(t, err)
				defer func() { assert.NoError(t, comp.Close()) }()
				c := comp.(*component)
				queried := false
				c.getRemappedRowsFunc = func(string, device.Device) (RemappedRows, error) { queried = true; return tc.rows, nil }
				cr := c.Check().(*checkResult)
				require.True(t, queried, "must actually query remapped rows on this product")
				if tc.action == "" {
					assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
					assert.Nil(t, cr.suggestedActions)
				} else {
					assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					require.NotNil(t, cr.suggestedActions)
					assert.Equal(t, []apiv1.RepairActionType{tc.action}, cr.suggestedActions.RepairActions)
				}
			})
		}
	}
}
