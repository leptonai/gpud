package scan

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	fabricmanager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	remappedrows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	"github.com/leptonai/gpud/components/all"
	pkgfirmware "github.com/leptonai/gpud/pkg/firmware"
	"github.com/leptonai/gpud/pkg/kmsg"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

type rubinScanDevice struct{ *testutil.MockDevice }

func (*rubinScanDevice) GetFabricState() (device.FabricState, error) {
	return device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.SUCCESS}, nil
}

type rubinScanInstance struct {
	nvidianvml.Instance
	dev device.Device
}

func (*rubinScanInstance) NVMLExists() bool           { return true }
func (*rubinScanInstance) ProductName() string        { return "NVIDIA Graphics Device" }
func (*rubinScanInstance) Architecture() string       { return "rubin" }
func (*rubinScanInstance) FabricStateSupported() bool { return true }
func (*rubinScanInstance) Capabilities() product.Capabilities {
	return product.CapabilitiesForGPU("NVIDIA Graphics Device", "rubin", product.HostLinkC2C)
}
func (i *rubinScanInstance) GetMemoryErrorManagementCapabilities() product.MemoryErrorManagementCapabilities {
	return i.Capabilities().MemoryManagement
}
func (i *rubinScanInstance) Devices() map[string]device.Device {
	return map[string]device.Device{"GPU-rubin": i.dev}
}

func TestScanGenericRubinRunsHealthChecks(t *testing.T) {
	for _, optIn := range []string{"", "true"} {
		t.Run("health-rules="+optIn, func(t *testing.T) {
			t.Setenv(product.EnvEnableRubinHealthRules, optIn)
			mockey.PatchConvey("one-shot scan runs real Rubin health components without a daemon", t, func() {
				dev := &rubinScanDevice{testutil.NewMockDevice(&mock.Device{
					GetBankRemapperStatus_v1Func: func() (nvml.EccBankRemapperStatus_v1, nvml.Return) {
						return nvml.EccBankRemapperStatus_v1{BPending: 1}, nvml.SUCCESS
					},
					GetRepairStatusFunc: func() (nvml.RepairStatus, nvml.Return) { return nvml.RepairStatus{}, nvml.ERROR_NOT_SUPPORTED },
					GetUnrepairableMemoryFlag_v1Func: func() (nvml.UnrepairableMemoryStatus_v1, nvml.Return) {
						return nvml.UnrepairableMemoryStatus_v1{}, nvml.ERROR_NOT_SUPPORTED
					},
					GetRemappedRowsFunc: func() (int, int, bool, bool, nvml.Return) { return 0, 0, false, false, nvml.SUCCESS },
				}, "rubin", "NVIDIA", "", "0000:01:00.0")}
				inst := &rubinScanInstance{Instance: nvidianvml.NewNoOp(), dev: dev}
				mockey.Mock(nvidianvml.New).Return(inst, nil).Build()
				mockey.Mock(pkgfirmware.RefreshWithin).Return(true).Build()
				mockey.Mock(pkgmachineinfo.GetMachineInfo).Return(&apiv1.MachineInfo{GPUInfo: &apiv1.MachineGPUInfo{Product: inst.ProductName(), Architecture: inst.Architecture()}}, nil).Build()
				mockey.Mock(kmsg.ReadAll).Return([]kmsg.Message{}, nil).Build()
				mockey.Mock(nvml.DeviceGetNvLinkState).To(func(_ nvml.Device, link int) (nvml.EnableState, nvml.Return) {
					if link == nvml.NVLINK_MAX_LINKS-1 {
						return nvml.FEATURE_DISABLED, nvml.SUCCESS
					}
					return nvml.FEATURE_ENABLED, nvml.SUCCESS
				}).Build()
				mockey.Mock(nvml.DeviceGetNvLinkErrorCounter).Return(uint64(0), nvml.SUCCESS).Build()
				// Use the production registry and real constructors for all Rubin-specific
				// polling components; isolate unrelated OS/services from this hardware fixture.
				var selected []all.Component
				for _, entry := range all.All() {
					switch entry.Name {
					case fabricmanager.Name, nvlink.Name, remappedrows.Name:
						init := entry.InitFunc
						entry.InitFunc = func(i *components.GPUdInstance) (components.Component, error) {
							c, err := init(i)
							if err == nil {
								t.Cleanup(func() { require.NoError(t, c.Close()) })
							}
							return c, err
						}
						selected = append(selected, entry)
					}
				}
				require.Len(t, selected, 3)
				mockey.Mock(all.All).Return(selected).Build()
				outputFile, err := os.CreateTemp(t.TempDir(), "scan-output")
				require.NoError(t, err)
				old := os.Stdout
				os.Stdout = outputFile
				defer func() { os.Stdout = old; _ = outputFile.Close() }()
				require.NoError(t, Scan(context.Background()))
				_, err = outputFile.Seek(0, 0)
				require.NoError(t, err)
				data, err := io.ReadAll(outputFile)
				require.NoError(t, err)
				output := string(data)
				for _, want := range []string{"rubin", "NVIDIA Graphics Device", "checked fabric state", "BANK REMAPPING PENDING", "N/A"} {
					require.Contains(t, output, want)
				}
				if optIn == "true" {
					for _, want := range []string{"links 35", "pending bank remapping", string(apiv1.RepairActionTypeRebootSystem)} {
						require.Contains(t, output, want)
					}
				} else {
					require.Contains(t, output, "telemetry only")
					require.NotContains(t, output, string(apiv1.RepairActionTypeRebootSystem))
					require.NotContains(t, output, "inactive or unverified nvlink")
				}
				require.NotContains(t, output, "executable not found")
			})
		})
	}
}
