package v1

import (
	"bytes"
	"testing"
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
				"/dev/nvme0n1p1", "part", "ext4", "537 GB", "/",
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
				"UUID", "SN", "MINORID",
				"GPU-abc123", "SN12345", "0",
				"/dev/nvme0n1p1", "part", "ext4", "537 GB", "/",
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
			wantContains: []string{"UUID", "SN", "MINORID", "GPU-abc123", "SN12345", "0"},
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
				"UUID", "SN", "MINORID",
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
				"/dev/nvme0n1p1", "part", "ext4", "537 GB", "/",
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
				"/dev/nvme0n1", "disk", "1.1 TB", "/dev/nvme0n1p1", "/dev/nvme0n1p2",
				"/dev/nvme0n1p1", "part", "ext4", "107 GB", "/boot", "/dev/nvme0n1",
				"/dev/nvme0n1p2", "part", "ext4", "966 GB", "/", "/dev/nvme0n1",
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
