package disk

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeviceUsages_RenderTable(t *testing.T) {
	tests := []struct {
		name     string
		devs     DeviceUsages
		expected []string
	}{
		{
			name: "empty device usages",
			devs: DeviceUsages{},
		},
		{
			name: "single device usage",
			devs: DeviceUsages{
				{
					DeviceName: "/dev/sda1",
					MountPoint: "/",
					TotalBytes: 1000000000,
					UsedBytes:  600000000,
					FreeBytes:  400000000,
				},
			},
			expected: []string{
				"/dev/sda1",
				"/",
				"954 MiB", // Total
				"572 MiB", // Used
				"382 MiB", // Free
			},
		},
		{
			name: "multiple device usages",
			devs: DeviceUsages{
				{
					DeviceName: "/dev/sda1",
					MountPoint: "/",
					TotalBytes: 1000000000,
					UsedBytes:  600000000,
					FreeBytes:  400000000,
				},
				{
					DeviceName: "/dev/sdb1",
					MountPoint: "/data",
					TotalBytes: 2000000000,
					UsedBytes:  1500000000,
					FreeBytes:  500000000,
				},
			},
			expected: []string{
				"/dev/sda1",
				"/dev/sdb1",
				"/data",
			},
		},
		{
			name: "device with zero bytes",
			devs: DeviceUsages{
				{
					DeviceName: "/dev/loop0",
					MountPoint: "/snap/core",
					TotalBytes: 0,
					UsedBytes:  0,
					FreeBytes:  0,
				},
			},
			expected: []string{
				"/dev/loop0",
				"/snap/core",
				"0 B",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.devs.RenderTable(&buf)
			output := buf.String()

			if len(tt.devs) == 0 {
				if output != "" {
					t.Errorf("Expected empty output for empty devices, got: %v", output)
				}
				return
			}

			// Verify that expected strings are in the output
			for _, exp := range tt.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("Expected output to contain %q, got:\n%v", exp, output)
				}
			}

			// Verify table header
			if len(tt.devs) > 0 {
				expectedHeaders := []string{"DEVICE", "MOUNT POINT", "TOTAL", "USED", "FREE"}
				for _, header := range expectedHeaders {
					if !strings.Contains(strings.ToUpper(output), header) {
						t.Errorf("Expected header %q in output, got:\n%v", header, output)
					}
				}
			}
		})
	}
}
