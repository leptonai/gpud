package pci

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestParseAccessControlServices(t *testing.T) {
	b, err := os.ReadFile("testdata/lspci-vvv")
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(b))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	devs, err := parseLspciVVV(ctx, scanner, func(name string) bool {
		return strings.Contains(name, "Mellanox Technologies")
	})
	if err != nil {
		t.Fatalf("failed to parse access control services: %v", err)
	}

	if len(devs) != 12 {
		t.Fatalf("expected 12 Mellanox devices, got %d", len(devs))
	}

	mellanoxACSCnt := 0
	for _, dev := range devs {
		if dev.KernelDriverInUse != "mlx5_core" {
			t.Errorf("device %q has wrong kernel driver in use: %v", dev.ID, dev.KernelDriverInUse)
		}
		if dev.KernelModules[0] != "mlx5_core" {
			t.Errorf("device %q has wrong kernel module: %v", dev.ID, dev.KernelModules)
		}

		if dev.AccessControlService == nil {
			continue
		}
		if strings.Contains(dev.Name, "Mellanox Technologies") {
			mellanoxACSCnt++
		}
	}

	// expects 4 "Ethernet controller: Mellanox Technologies" with "Access Control Services"
	if mellanoxACSCnt != 4 {
		t.Fatalf("expected 4 Mellanox devices, got %d", mellanoxACSCnt)
	}

	yb, err := devs.YAML()
	if err != nil {
		t.Fatalf("failed to marshal to YAML: %v", err)
	}
	t.Logf("yaml: %s", string(yb))
}

func TestPCIDeviceHeaderRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid PCI device header",
			input: "19:00.0 3D controller: NVIDIA Corporation Device 2330 (rev a1)",
			want:  true,
		},
		{
			name:  "valid PCI bridge header",
			input: "17:1f.0 PCI bridge: Broadcom / LSI PEX890xx PCIe Gen 5 Switch (rev b0)",
			want:  true,
		},
		{
			name:  "valid system peripheral header",
			input: "ff:0c.1 System peripheral: Intel Corporation Device 324d",
			want:  true,
		},
		{
			name:  "valid system peripheral header with no revision",
			input: "ff:0c.2 System peripheral: Intel Corporation Device 324d",
			want:  true,
		},
		{
			name:  "invalid format - no colon",
			input: "1900.0 3D controller NVIDIA Corporation Device 2330",
			want:  false,
		},
		{
			name:  "invalid format - leading space",
			input: " 19:00.0 3D controller: NVIDIA Corporation Device 2330",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pciDeviceHeaderRegexCompiled.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("pciDeviceHeaderRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestKernelModulesRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "nvidia modules",
			input: "	Kernel modules: nvidiafb, nouveau, nvidia_drm, nvidia",
			want:  true,
		},
		{
			name:  "single module",
			input: "	Kernel modules: mpt3sas",
			want:  true,
		},
		{
			name:  "no modules listed",
			input: "	Kernel modules:",
			want:  false,
		},
		{
			name:  "invalid format - missing prefix",
			input: "nvidiafb, nouveau, nvidia_drm, nvidia",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kernelModulesRegexCompiled.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("kernelModulesRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestKernelDriverInUseRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "nvidia driver",
			input: "	Kernel driver in use: nvidia",
			want:  true,
		},
		{
			name:  "pcieport driver",
			input: "	Kernel driver in use: pcieport",
			want:  true,
		},
		{
			name:  "invalid format - missing prefix",
			input: "nvidia",
			want:  false,
		},
		{
			name:  "extra text",
			input: "	Kernel driver in use: nvidia extra",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kernelDriverInUseRegexCompiled.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("kernelDriverInUseRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCapAccessControlServicesRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid ACS capability",
			input: "	Capabilities: [170 v1] Access Control Services",
			want:  true,
		},
		{
			name:  "different address",
			input: "	Capabilities: [220 v1] Access Control Services",
			want:  true,
		},
		{
			name:  "invalid format - missing version",
			input: "	Capabilities: [170] Access Control Services",
			want:  false,
		},
		{
			name:  "invalid format - wrong capability",
			input: "	Capabilities: [170 v1] Power Management",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capAccessControlServicesRegexCompiled.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("capAccessControlServicesRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseACS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ACS
	}{
		{
			name:  "all enabled",
			input: "SrcValid+ TransBlk+ ReqRedir+ CmpltRedir+ UpstreamFwd+ EgressCtrl+ DirectTrans+",
			expected: ACS{
				SrcValid:    true,
				TransBlk:    true,
				ReqRedir:    true,
				CmpltRedir:  true,
				UpstreamFwd: true,
				EgressCtrl:  true,
				DirectTrans: true,
			},
		},
		{
			name:  "all disabled",
			input: "SrcValid- TransBlk- ReqRedir- CmpltRedir- UpstreamFwd- EgressCtrl- DirectTrans-",
			expected: ACS{
				SrcValid:    false,
				TransBlk:    false,
				ReqRedir:    false,
				CmpltRedir:  false,
				UpstreamFwd: false,
				EgressCtrl:  false,
				DirectTrans: false,
			},
		},
		{
			name:  "mixed states",
			input: "SrcValid+ TransBlk+ ReqRedir- CmpltRedir- UpstreamFwd+ EgressCtrl- DirectTrans-",
			expected: ACS{
				SrcValid:    true,
				TransBlk:    true,
				ReqRedir:    false,
				CmpltRedir:  false,
				UpstreamFwd: true,
				EgressCtrl:  false,
				DirectTrans: false,
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: ACS{},
		},
		{
			name:     "malformed input without +/-",
			input:    "SrcValid TransBlk ReqRedir",
			expected: ACS{},
		},
		{
			name:  "with prefix and extra whitespace",
			input: "ACSCap:\tSrcValid+ TransBlk+    ReqRedir-",
			expected: ACS{
				SrcValid: true,
				TransBlk: true,
				ReqRedir: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseACS(tt.input)
			if got != tt.expected {
				t.Errorf("ParseACS(%q) = %+v, want %+v", tt.input, got, tt.expected)
			}
		})
	}
}
