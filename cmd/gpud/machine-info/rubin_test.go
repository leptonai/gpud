package machineinfo

import (
	"encoding/json"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	pkgfirmware "github.com/leptonai/gpud/pkg/firmware"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/providers"
)

type rubinMachineInstance struct {
	nvidianvml.Instance
	dev device.Device
}

func (*rubinMachineInstance) ProductName() string  { return "NVIDIA Graphics Device" }
func (*rubinMachineInstance) Architecture() string { return "rubin" }
func (*rubinMachineInstance) Brand() string        { return "NVIDIA" }
func (i *rubinMachineInstance) Devices() map[string]device.Device {
	return map[string]device.Device{"GPU-rubin": i.dev}
}

func TestCommandRubinInventory(t *testing.T) {
	for _, format := range []string{gpudcommon.OutputFormatPlain, gpudcommon.OutputFormatJSON} {
		t.Run(format, func(t *testing.T) {
			mockey.PatchConvey("generic Rubin inventory reaches both command output formats", t, func() {
				mockey.Mock(gpudcommon.StateFileFromContext).Return(t.TempDir()+"/state.db", nil).Build()
				dev := testutil.NewMockDeviceWithIDs(&mock.Device{
					GetMemoryInfo_v2Func: func() (nvml.Memory_v2, nvml.Return) { return nvml.Memory_v2{Total: 1 << 38}, nvml.SUCCESS },
				}, "rubin", "NVIDIA", "", "0000:01:00.0", "GPU-rubin", "VR-SERIAL", 0, 1)
				inst := &rubinMachineInstance{Instance: nvidianvml.NewNoOp(), dev: dev}
				mockey.Mock(nvidianvml.New).Return(inst, nil).Build()
				mockey.Mock(pkgfirmware.RefreshWithin).Return(true).Build()
				// Leave GPU inventory collection and rendering real; isolate unrelated host
				// and provider probes so this test never requires GPU hardware or networking.
				mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(i nvidianvml.Instance) (*apiv1.MachineInfo, error) {
					gpu, err := pkgmachineinfo.GetMachineGPUInfo(i)
					return &apiv1.MachineInfo{GPUInfo: gpu}, err
				}).Build()
				mockey.Mock(netutil.PublicIP).Return("", nil).Build()
				mockey.Mock(pkgmachineinfo.GetProvider).To(func(string) *providers.Info { return nil }).Build()
				cliContext := newCLIContext(t, []string{"--state-file", t.TempDir() + "/state.db", "--output-format", format})
				stdout, _ := captureOutput(t, func() { require.NoError(t, Command(cliContext)) })
				if format == gpudcommon.OutputFormatJSON {
					var payload struct {
						MachineInfo apiv1.MachineInfo `json:"machine_info"`
					}
					require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
					require.Equal(t, "rubin", payload.MachineInfo.GPUInfo.Architecture)
					require.Equal(t, "NVIDIA Graphics Device", payload.MachineInfo.GPUInfo.Product)
					require.Len(t, payload.MachineInfo.GPUInfo.GPUs, 1)
					require.Equal(t, "GPU-rubin", payload.MachineInfo.GPUInfo.GPUs[0].UUID)
					require.Equal(t, "0000:01:00.0", payload.MachineInfo.GPUInfo.GPUs[0].BusID)
					require.NotEmpty(t, payload.MachineInfo.GPUInfo.Memory)
				} else {
					for _, value := range []string{"GPU Architecture", "rubin", "NVIDIA Graphics Device", "GPU-rubin", "0000:01:00.0", "VR-SERIAL"} {
						require.Contains(t, stdout, value)
					}
				}
			})
		})
	}
}
