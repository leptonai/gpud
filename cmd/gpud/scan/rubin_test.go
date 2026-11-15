package scan

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/components/all"
	pkgfirmware "github.com/leptonai/gpud/pkg/firmware"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
)

func TestScanRubinExplicitNICInventory(t *testing.T) {
	saved := infiniband.GetDefaultExpectedPortStates()
	t.Cleanup(func() { infiniband.SetDefaultExpectedPortStates(saved) })
	for _, product := range []string{"NVIDIA Graphics Device", "NVIDIA Vera Rubin NVL72"} {
		for _, tc := range []struct {
			name, config string
			want         types.ExpectedPortStates
		}{
			{name: "actual dev inventory", config: `{"at_least_ports":2,"at_least_rate":400}`, want: types.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400}},
			{name: "explicitly disabled", config: `{}`},
		} {
			t.Run(product+"/"+tc.name, func(t *testing.T) {
				mockey.PatchConvey("CLI inventory survives product default selection", t, func() {
					mockey.Mock(nvidianvml.New).Return(nvidianvml.NewNoOp(), nil).Build()
					mockey.Mock(pkgfirmware.RefreshWithin).Return(true).Build()
					mockey.Mock(pkgmachineinfo.GetMachineInfo).Return(&apiv1.MachineInfo{GPUInfo: &apiv1.MachineGPUInfo{Product: product, Architecture: "rubin"}}, nil).Build()
					mockey.Mock(pkgmachineinfo.GetProviderWithContext).To(func(context.Context, string) *providers.Info { return nil }).Build()
					checked := false
					mockey.Mock(all.All).Return([]all.Component{{Name: "threshold-probe", InitFunc: func(*components.GPUdInstance) (components.Component, error) {
						checked = true
						require.Equal(t, tc.want, infiniband.GetDefaultExpectedPortStates())
						return nil, context.Canceled // Stop after inspecting the real scan configuration.
					}}}).Build()
					err := cmdScan("error", 0, tc.config, "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false)
					require.ErrorIs(t, err, context.Canceled)
					require.True(t, checked)
				})
			})
		}
	}
}
