package machineinfo

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentcontainerd "github.com/leptonai/gpud/components/containerd"
	componenttailscale "github.com/leptonai/gpud/components/tailscale"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
)

func TestMachineInfoConfiguredContainerd(t *testing.T) {
	for _, running := range []bool{false, true} {
		name := "down"
		if running {
			name = "running"
		}
		t.Run(name, func(t *testing.T) {
			mockey.PatchConvey("configured endpoint is authoritative", t, func() {
				cfg := configcommon.ContainerdConfig{Endpoint: "unix:///run/k3s/containerd/containerd.sock"}
				mockey.Mock(currentGOOS).Return("linux").Build()
				mockey.Mock(GetMachineGPUInfo).Return(&apiv1.MachineGPUInfo{}, nil).Build()
				mockey.Mock(GetMachineDiskInfo).Return(&apiv1.MachineDiskInfo{}, nil).Build()
				mockey.Mock(componenttailscale.CheckTailscaleInstalled).Return(false).Build()
				mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { t.Fatal("must not inspect image binary"); return false }).Build()
				mockey.Mock(componentcontainerd.CheckCRIORunning).To(func(context.Context) bool { t.Fatal("must not fall back to CRI-O"); return true }).Build()
				mockey.Mock(componentcontainerd.CheckContainerdRunningAt).To(func(_ context.Context, endpoint string) bool {
					require.Equal(t, cfg.Endpoint, endpoint)
					return running
				}).Build()
				mockey.Mock(componentcontainerd.GetVersion).To(func(_ context.Context, endpoint string) (string, error) {
					require.Equal(t, cfg.Endpoint, endpoint)
					return "2.0.0", nil
				}).Build()
				info, err := GetMachineInfoWithContainerd(&mockNvmlInstanceForMockey{}, cfg)
				require.NoError(t, err)
				if running {
					require.Equal(t, "containerd://2.0.0", info.ContainerRuntimeVersion)
				} else {
					require.Empty(t, info.ContainerRuntimeVersion)
				}
			})
		})
	}
}

func TestGossipConfiguredContainerd(t *testing.T) {
	mockey.PatchConvey("gossip forwards runtime configuration", t, func() {
		cfg := configcommon.ContainerdConfig{ServiceName: "rke2-agent"}
		mockey.Mock(GetMachineInfoWithContainerd).To(func(_ nvidianvml.Instance, got configcommon.ContainerdConfig) (*apiv1.MachineInfo, error) {
			require.Equal(t, cfg, got)
			return &apiv1.MachineInfo{ContainerRuntimeVersion: "containerd://2.0.0"}, nil
		}).Build()
		req, err := CreateGossipRequestWithContainerd("machine", nil, cfg)
		require.NoError(t, err)
		require.Equal(t, "containerd://2.0.0", req.MachineInfo.ContainerRuntimeVersion)
	})
}
