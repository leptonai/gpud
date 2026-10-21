package v1

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestEventTypeFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected EventType
	}{
		{
			name:     "Info event type",
			input:    "Info",
			expected: EventTypeInfo,
		},
		{
			name:     "Warning event type",
			input:    "Warning",
			expected: EventTypeWarning,
		},
		{
			name:     "Critical event type",
			input:    "Critical",
			expected: EventTypeCritical,
		},
		{
			name:     "Fatal event type",
			input:    "Fatal",
			expected: EventTypeFatal,
		},
		{
			name:     "Unknown event type",
			input:    "NonExistent",
			expected: EventTypeUnknown,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: EventTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EventTypeFromString(tt.input)
			if got != tt.expected {
				t.Errorf("EventTypeFromString(%q) = %v, want %v",
					tt.input, got, tt.expected)
			}
		})
	}
}

func TestSuggestedActions_DescribeActions(t *testing.T) {
	tests := []struct {
		name             string
		suggestedActions SuggestedActions
		expected         string
	}{
		{
			name: "Single repair action",
			suggestedActions: SuggestedActions{
				Description:   "Test description",
				RepairActions: []RepairActionType{RepairActionTypeRebootSystem},
			},
			expected: "REBOOT_SYSTEM",
		},
		{
			name: "Multiple repair actions",
			suggestedActions: SuggestedActions{
				Description:   "Multiple actions needed",
				RepairActions: []RepairActionType{RepairActionTypeRebootSystem, RepairActionTypeHardwareInspection},
			},
			expected: "REBOOT_SYSTEM, HARDWARE_INSPECTION",
		},
		{
			name: "No repair actions",
			suggestedActions: SuggestedActions{
				Description:   "No actions needed",
				RepairActions: []RepairActionType{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.suggestedActions.DescribeActions()
			if got != tt.expected {
				t.Errorf("SuggestedActions.DescribeActions() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMachineInfo_GPUIdentifierJSONShape(t *testing.T) {
	cliqueZero := uint32(0)
	info := MachineInfo{
		GPUInfo: &MachineGPUInfo{
			ClusterUUID:   "12345678-1234-1234-1234-1234567890ab",
			CliqueID:      &cliqueZero,
			ChassisSerial: "3136434J5234567",
		},
	}

	raw, err := json.Marshal(info)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.NotContains(t, got, "clusterUUID")
	assert.NotContains(t, got, "cliqueID")
	assert.NotContains(t, got, "chassisSerial")
	gpuInfo, ok := got["gpuInfo"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "12345678-1234-1234-1234-1234567890ab", gpuInfo["clusterUUID"])
	assert.Equal(t, float64(0), gpuInfo["cliqueID"])
	assert.Equal(t, "3136434J5234567", gpuInfo["chassisSerial"])

	info.GPUInfo.CliqueID = nil
	raw, err = json.Marshal(info)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "cliqueID")
}

// firmwareFixtureJSON is the GPUd wire shape of MachineInfo.Firmware with every
// component. The Lepton gpud-manager decodes the same text into the Machine CRD.
const firmwareFixtureJSON = `{"firmware":[` +
	`{"component":"GPU","version":"97.10.4A.00.0B","instances":4,"instanceNames":["gpu1","gpu2","gpu3","gpu4"],"lastUpdated":"2026-10-21T08:00:00Z"},` +
	`{"component":"bf3_nic","version":"32.47.2526","instances":1,"instanceNames":["mlx5_bond_0"],"lastUpdated":"2026-10-21T08:00:00Z"},` +
	`{"component":"cx8","version":"40.47.2526","instances":4,"instanceNames":["rdma_rail0","rdma_rail1","rdma_rail2","rdma_rail3"],"lastUpdated":"2026-10-21T08:00:00Z"},` +
	`{"component":"cx8-aws","version":"40.47.2526","instances":1,"instanceNames":["mlx5_0"],"lastUpdated":"2026-10-21T08:00:00Z"},` +
	`{"component":"doca_host","version":"3.2.1-044413","instances":1,"lastUpdated":"2026-10-21T08:00:00Z"},` +
	`{"component":"hmc-sbios","version":"5.36_O72OR37A_02.05.05","instances":1,"lastUpdated":"2026-10-21T08:00:00Z"}` +
	`]}`

func TestMachineInfo_FirmwareJSONShape(t *testing.T) {
	var info MachineInfo
	require.NoError(t, json.Unmarshal([]byte(firmwareFixtureJSON), &info))
	require.Len(t, info.Firmware, 6)

	gpu := info.Firmware[0]
	assert.Equal(t, FirmwareComponentGPU, gpu.Component)
	assert.Equal(t, "97.10.4A.00.0B", gpu.Version)
	assert.Equal(t, int32(4), gpu.Instances)
	assert.Equal(t, []string{"gpu1", "gpu2", "gpu3", "gpu4"}, gpu.InstanceNames)
	assert.True(t, gpu.LastUpdated.Equal(&metav1.Time{Time: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)}))

	components := make([]string, 0, len(info.Firmware))
	for _, fw := range info.Firmware {
		components = append(components, fw.Component)
	}
	assert.Equal(t, []string{
		FirmwareComponentGPU,
		FirmwareComponentBF3NIC,
		FirmwareComponentCX8,
		FirmwareComponentCX8AWS,
		FirmwareComponentDOCAHost,
		FirmwareComponentHMCSBIOS,
	}, components)
	assert.Nil(t, info.Firmware[4].InstanceNames)

	// MachineInfo always marshals uptime, so compare only the firmware key.
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	var want, roundTrip map[string]any
	require.NoError(t, json.Unmarshal([]byte(firmwareFixtureJSON), &want))
	require.NoError(t, json.Unmarshal(raw, &roundTrip))
	assert.Equal(t, want["firmware"], roundTrip["firmware"])

	// A time in another zone with sub-second precision still goes out as whole-second UTC.
	info = MachineInfo{Firmware: []MachineFirmware{{
		Component:   FirmwareComponentDOCAHost,
		Version:     "3.2.1-044413",
		Instances:   1,
		LastUpdated: metav1.NewTime(time.Date(2026, 10, 21, 16, 0, 0, 999, time.FixedZone("UTC+8", 8*3600))),
	}}}
	raw, err = json.Marshal(info)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	rows, ok := got["firmware"].([]any)
	require.True(t, ok)
	require.Len(t, rows, 1)
	row, ok := rows[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"component":   "doca_host",
		"version":     "3.2.1-044413",
		"instances":   float64(1),
		"lastUpdated": "2026-10-21T08:00:00Z",
	}, row)

	raw, err = json.Marshal(MachineInfo{GPUdVersion: "v0.0.1"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "firmware")
}

func TestMachineInfo_RenderTable(t *testing.T) {
	tests := []struct {
		name         string
		machineInfo  MachineInfo
		wantContains []string
	}{
		{
			name: "Basic machine info",
			machineInfo: MachineInfo{
				GPUdVersion:             "1.0.0",
				CUDAVersion:             "11.7.0",
				ContainerRuntimeVersion: "containerd://1.4.2",
				KernelVersion:           "5.4.0-1024-aws",
				OSImage:                 "Ubuntu 20.04.3 LTS",
				GPUDriverVersion:        "470.82.01",
			},
			wantContains: []string{
				"GPUd Version", "1.0.0",
				"CUDA Version", "11.7.0",
				"Container Runtime Version", "containerd://1.4.2",
				"Kernel Version", "5.4.0-1024-aws",
				"OS Image", "Ubuntu 20.04.3 LTS",
			},
		},
		{
			name: "Machine info with GPU info",
			machineInfo: MachineInfo{
				GPUdVersion:      "1.0.0",
				GPUDriverVersion: "470.82.01",
				GPUInfo: &MachineGPUInfo{
					Product:      "NVIDIA A100-SXM4-40GB",
					Manufacturer: "NVIDIA",
					Architecture: "ampere",
					Memory:       "40GB",
				},
			},
			wantContains: []string{
				"GPUd Version", "1.0.0",
				"GPU Driver Version", "470.82.01",
				"GPU Product", "NVIDIA A100-SXM4-40GB",
				"GPU Manufacturer", "NVIDIA",
				"GPU Architecture", "ampere",
				"GPU Memory", "40GB",
			},
		},
		{
			name: "Machine info with GPU fabric identifiers and chassis serial",
			machineInfo: MachineInfo{
				GPUdVersion: "1.0.0",
				GPUInfo: &MachineGPUInfo{
					ClusterUUID:   "12345678-1234-1234-1234-1234567890ab",
					CliqueID:      ptr.To(uint32(42)),
					ChassisSerial: "3136434J5234567",
				},
			},
			wantContains: []string{
				"GPUd Version", "1.0.0",
				"GPU Fabric Cluster UUID", "12345678-1234-1234-1234-1234567890ab",
				"GPU Fabric Clique ID", "42",
				"Chassis Serial", "3136434J5234567",
			},
		},
		{
			name: "Machine info with Disk info",
			machineInfo: MachineInfo{
				GPUdVersion:   "1.0.0",
				KernelVersion: "5.4.0-1024-aws",
				OSImage:       "Ubuntu 20.04.3 LTS",
				DiskInfo: &MachineDiskInfo{
					ContainerRootDisk: "/dev/nvme0n1p1",
					BlockDevices: []MachineDiskDevice{
						{
							Name:       "/dev/nvme0n1p1",
							Type:       "part",
							FSType:     "ext4",
							Size:       500 * 1024 * 1024 * 1024,
							MountPoint: "/",
						},
					},
				},
			},
			wantContains: []string{
				"GPUd Version", "1.0.0",
				"Kernel Version", "5.4.0-1024-aws",
				"OS Image", "Ubuntu 20.04.3 LTS",
				"Container Root Disk", "/dev/nvme0n1p1",
				"NAME", "TYPE", "FSTYPE", "SIZE", "MOUNT POINT",
				"/dev/nvme0n1p1", "part", "ext4", "500 GiB", "/",
			},
		},
		{
			name: "Machine info with both GPU and Disk info",
			machineInfo: MachineInfo{
				GPUdVersion:      "1.0.0",
				GPUDriverVersion: "470.82.01",
				GPUInfo: &MachineGPUInfo{
					Product:      "NVIDIA A100-SXM4-40GB",
					Manufacturer: "NVIDIA",
					Architecture: "ampere",
					Memory:       "40GB",
					GPUs: []MachineGPUInstance{
						{
							UUID:    "GPU-abc123",
							SN:      "SN12345",
							MinorID: "0",
						},
					},
				},
				DiskInfo: &MachineDiskInfo{
					ContainerRootDisk: "/dev/nvme0n1p1",
					BlockDevices: []MachineDiskDevice{
						{
							Name:       "/dev/nvme0n1p1",
							Type:       "part",
							FSType:     "ext4",
							Size:       500 * 1024 * 1024 * 1024,
							MountPoint: "/",
						},
					},
				},
			},
			wantContains: []string{
				"GPUd Version", "1.0.0",
				"GPU Driver Version", "470.82.01",
				"GPU Product", "NVIDIA A100-SXM4-40GB",
				"Container Root Disk", "/dev/nvme0n1p1",
				"UUID", "SN", "MINOR ID",
				"GPU-abc123", "SN12345", "0",
				"/dev/nvme0n1p1", "part", "ext4", "500 GiB", "/",
			},
		},
		{
			name: "Machine info with firmware",
			machineInfo: MachineInfo{
				GPUdVersion: "1.0.0",
				Firmware: []MachineFirmware{
					{
						Component:     FirmwareComponentGPU,
						Version:       "97.10.4A.00.0B",
						Instances:     2,
						InstanceNames: []string{"gpu1", "gpu2"},
						LastUpdated:   metav1.NewTime(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)),
					},
					{
						Component:   FirmwareComponentHMCSBIOS,
						Version:     "5.36_O72OR37A_02.05.05",
						Instances:   1,
						LastUpdated: metav1.NewTime(time.Date(2026, 10, 21, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))),
					},
				},
			},
			wantContains: []string{
				"COMPONENT", "VERSION", "INSTANCES", "INSTANCE NAMES", "LAST UPDATED (UTC)",
				"GPU", "97.10.4A.00.0B", "gpu1, gpu2", "2026-10-21T08:00:00Z",
				"hmc-sbios", "5.36_O72OR37A_02.05.05", "2026-10-21T00:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.machineInfo.RenderTable(&buf)
			rendered := buf.String()

			for _, wantStr := range tt.wantContains {
				if !bytes.Contains(buf.Bytes(), []byte(wantStr)) {
					t.Errorf("MachineInfo.RenderTable() output does not contain %q\nGot: %q", wantStr, rendered)
				}
			}
		})
	}
}

func TestMachineGPUInfo_RenderTable(t *testing.T) {
	tests := []struct {
		name         string
		gpuInfo      MachineGPUInfo
		wantContains []string
	}{
		{
			name: "No GPUs",
			gpuInfo: MachineGPUInfo{
				Product:      "NVIDIA A100-SXM4-40GB",
				Manufacturer: "NVIDIA",
				Architecture: "ampere",
				Memory:       "40GB",
				GPUs:         []MachineGPUInstance{},
			},
			wantContains: []string{},
		},
		{
			name: "Single GPU",
			gpuInfo: MachineGPUInfo{
				Product:      "NVIDIA A100-SXM4-40GB",
				Manufacturer: "NVIDIA",
				Architecture: "ampere",
				Memory:       "40GB",
				GPUs: []MachineGPUInstance{
					{
						UUID:    "GPU-abc123",
						SN:      "SN12345",
						MinorID: "0",
					},
				},
			},
			wantContains: []string{"UUID", "SN", "MINOR ID", "GPU-abc123", "SN12345", "0"},
		},
		{
			name: "Multiple GPUs",
			gpuInfo: MachineGPUInfo{
				Product:      "NVIDIA A100-SXM4-40GB",
				Manufacturer: "NVIDIA",
				Architecture: "ampere",
				Memory:       "40GB",
				GPUs: []MachineGPUInstance{
					{
						UUID:    "GPU-abc123",
						SN:      "SN12345",
						MinorID: "0",
					},
					{
						UUID:    "GPU-def456",
						SN:      "SN67890",
						MinorID: "1",
					},
				},
			},
			wantContains: []string{
				"UUID", "SN", "MINOR ID",
				"GPU-abc123", "SN12345", "0",
				"GPU-def456", "SN67890", "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.gpuInfo.RenderTable(&buf)
			rendered := buf.String()

			// If we don't expect any output for cases like no GPUs
			if len(tt.wantContains) == 0 {
				if rendered != "" {
					t.Errorf("MachineGPUInfo.RenderTable() expected no output for empty GPUs list, got: %q", rendered)
				}
				return
			}

			for _, wantStr := range tt.wantContains {
				if !bytes.Contains(buf.Bytes(), []byte(wantStr)) {
					t.Errorf("MachineGPUInfo.RenderTable() output does not contain %q\nGot: %q", wantStr, rendered)
				}
			}
		})
	}
}

func TestMachineDiskInfo_RenderTable(t *testing.T) {
	tests := []struct {
		name         string
		diskInfo     MachineDiskInfo
		wantContains []string
	}{
		{
			name: "No block devices",
			diskInfo: MachineDiskInfo{
				ContainerRootDisk: "/dev/nvme0n1p1",
				BlockDevices:      []MachineDiskDevice{},
			},
			wantContains: []string{},
		},
		{
			name: "Single block device",
			diskInfo: MachineDiskInfo{
				ContainerRootDisk: "/dev/nvme0n1p1",
				BlockDevices: []MachineDiskDevice{
					{
						Name:       "/dev/nvme0n1p1",
						Type:       "part",
						FSType:     "ext4",
						Size:       500 * 1024 * 1024 * 1024, // 500GB
						MountPoint: "/",
					},
				},
			},
			wantContains: []string{
				"NAME", "TYPE", "FSTYPE", "SIZE", "MOUNT POINT", "PARENTS", "CHILDREN",
				"/dev/nvme0n1p1", "part", "ext4", "500 GiB", "/",
			},
		},
		{
			name: "Multiple block devices with parent-child relationships",
			diskInfo: MachineDiskInfo{
				ContainerRootDisk: "/dev/nvme0n1p2",
				BlockDevices: []MachineDiskDevice{
					{
						Name:     "/dev/nvme0n1",
						Type:     "disk",
						Size:     1000 * 1024 * 1024 * 1024, // 1TB
						Children: []string{"/dev/nvme0n1p1", "/dev/nvme0n1p2"},
					},
					{
						Name:       "/dev/nvme0n1p1",
						Type:       "part",
						FSType:     "ext4",
						Size:       100 * 1024 * 1024 * 1024, // 100GB
						MountPoint: "/boot",
						Parents:    []string{"/dev/nvme0n1"},
					},
					{
						Name:       "/dev/nvme0n1p2",
						Type:       "part",
						FSType:     "ext4",
						Size:       900 * 1024 * 1024 * 1024, // 900GB
						MountPoint: "/",
						Parents:    []string{"/dev/nvme0n1"},
					},
				},
			},
			wantContains: []string{
				"NAME", "TYPE", "FSTYPE", "SIZE", "MOUNT POINT", "PARENTS", "CHILDREN",
				"/dev/nvme0n1", "disk", "1000 GiB", "/dev/nvme0n1p1", "/dev/nvme0n1p2",
				"/dev/nvme0n1p1", "part", "ext4", "100 GiB", "/boot", "/dev/nvme0n1",
				"/dev/nvme0n1p2", "part", "ext4", "900 GiB", "/", "/dev/nvme0n1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.diskInfo.RenderTable(&buf)
			rendered := buf.String()

			// If we don't expect any output for cases like no block devices
			if len(tt.wantContains) == 0 {
				if rendered != "" {
					t.Errorf("MachineDiskInfo.RenderTable() expected no output for empty block devices list, got: %q", rendered)
				}
				return
			}

			for _, wantStr := range tt.wantContains {
				if !bytes.Contains(buf.Bytes(), []byte(wantStr)) {
					t.Errorf("MachineDiskInfo.RenderTable() output does not contain %q\nGot: %q", wantStr, rendered)
				}
			}
		})
	}
}

func TestMachineCPUInfo_TopologyJSON(t *testing.T) {
	tests := []struct {
		name string
		info MachineCPUInfo
		want string
	}{
		{
			name: "measured topology",
			info: MachineCPUInfo{LogicalCores: 16, CPUsPerCore: 2, NumCores: 8, NumSockets: 2},
			want: `{"logicalCores":16,"cpusPerCore":2,"numCores":8,"numSockets":2}`,
		},
		{
			name: "unknown topology stays omitted",
			info: MachineCPUInfo{LogicalCores: 16},
			want: `{"logicalCores":16}`,
		},
		{
			name: "nonuniform online threads omit only multiplier",
			info: MachineCPUInfo{LogicalCores: 3, NumCores: 2, NumSockets: 1},
			want: `{"logicalCores":3,"numCores":2,"numSockets":1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.info)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
			var decoded MachineCPUInfo
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tt.info, decoded)
		})
	}
}
