package disk

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlatten(t *testing.T) {
	t.Parallel()

	for f, expectedDevs := range map[string]int{
		"lsblk.1.json": 23,
		"lsblk.2.json": 10,
		"lsblk.3.json": 20,
		"lsblk.4.json": 39,
		"lsblk.5.json": 11,
	} {
		dat, err := os.ReadFile("testdata/" + f)
		require.NoError(t, err)

		blks, err := parseLsblkJSON(context.Background(), dat)
		require.NoError(t, err)

		flattened := blks.Flatten()
		require.Equal(t, expectedDevs, len(flattened))

		flattened.RenderTable(os.Stdout)
	}
}

func TestFlattenedBlockDevices_GetDeviceUsages(t *testing.T) {
	tests := []struct {
		name      string
		blks      FlattenedBlockDevices
		parts     Partitions
		wantCount int
		validate  func(t *testing.T, got DeviceUsages)
	}{
		{
			name:      "empty block devices and partitions",
			blks:      FlattenedBlockDevices{},
			parts:     Partitions{},
			wantCount: 0,
		},
		{
			name: "block devices with no mount points",
			blks: FlattenedBlockDevices{
				{
					Name:       "/dev/sda",
					Type:       "disk",
					MountPoint: "",
				},
				{
					Name:       "/dev/sdb",
					Type:       "disk",
					MountPoint: "",
				},
			},
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &Usage{
						TotalBytes: 1000000,
						UsedBytes:  500000,
						FreeBytes:  500000,
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "partitions with nil usage",
			blks: FlattenedBlockDevices{
				{
					Name:       "/dev/sda1",
					Type:       "part",
					MountPoint: "/",
				},
			},
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage:      nil,
				},
			},
			wantCount: 0,
		},
		{
			name: "matching mount points between blocks and partitions",
			blks: FlattenedBlockDevices{
				{
					Name:       "/dev/sda1",
					Type:       "part",
					MountPoint: "/",
				},
				{
					Name:       "/dev/sdb1",
					Type:       "part",
					MountPoint: "/home",
				},
			},
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &Usage{
						TotalBytes: 1000000,
						UsedBytes:  600000,
						FreeBytes:  400000,
					},
				},
				{
					Device:     "/dev/sdb1",
					MountPoint: "/home",
					Usage: &Usage{
						TotalBytes: 2000000,
						UsedBytes:  1000000,
						FreeBytes:  1000000,
					},
				},
			},
			wantCount: 2,
			validate: func(t *testing.T, got DeviceUsages) {
				if len(got) != 2 {
					t.Fatalf("Expected 2 device usages, got %d", len(got))
				}

				// Check first device
				if got[0].DeviceName != "/dev/sda1" {
					t.Errorf("Expected DeviceName /dev/sda1, got %s", got[0].DeviceName)
				}
				if got[0].MountPoint != "/" {
					t.Errorf("Expected MountPoint /, got %s", got[0].MountPoint)
				}
				if got[0].TotalBytes != 1000000 {
					t.Errorf("Expected TotalBytes 1000000, got %d", got[0].TotalBytes)
				}
				if got[0].UsedBytes != 600000 {
					t.Errorf("Expected UsedBytes 600000, got %d", got[0].UsedBytes)
				}
				if got[0].FreeBytes != 400000 {
					t.Errorf("Expected FreeBytes 400000, got %d", got[0].FreeBytes)
				}

				// Check second device
				if got[1].DeviceName != "/dev/sdb1" {
					t.Errorf("Expected DeviceName /dev/sdb1, got %s", got[1].DeviceName)
				}
				if got[1].MountPoint != "/home" {
					t.Errorf("Expected MountPoint /home, got %s", got[1].MountPoint)
				}
				if got[1].TotalBytes != 2000000 {
					t.Errorf("Expected TotalBytes 2000000, got %d", got[1].TotalBytes)
				}
				if got[1].UsedBytes != 1000000 {
					t.Errorf("Expected UsedBytes 1000000, got %d", got[1].UsedBytes)
				}
				if got[1].FreeBytes != 1000000 {
					t.Errorf("Expected FreeBytes 1000000, got %d", got[1].FreeBytes)
				}
			},
		},
		{
			name: "mismatched mount points",
			blks: FlattenedBlockDevices{
				{
					Name:       "/dev/sda1",
					Type:       "part",
					MountPoint: "/boot",
				},
			},
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &Usage{
						TotalBytes: 1000000,
						UsedBytes:  600000,
						FreeBytes:  400000,
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "multiple blocks same mount point takes first match",
			blks: FlattenedBlockDevices{
				{
					Name:       "/dev/sda1",
					Type:       "part",
					MountPoint: "/",
				},
				{
					Name:       "/dev/sda2",
					Type:       "part",
					MountPoint: "/data",
				},
				{
					Name:       "/dev/sdb1",
					Type:       "part",
					MountPoint: "/",
				},
			},
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &Usage{
						TotalBytes: 1000000,
						UsedBytes:  500000,
						FreeBytes:  500000,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/data",
					Usage: &Usage{
						TotalBytes: 2000000,
						UsedBytes:  1500000,
						FreeBytes:  500000,
					},
				},
			},
			wantCount: 3,
			validate: func(t *testing.T, got DeviceUsages) {
				if len(got) != 3 {
					t.Fatalf("Expected 3 device usages, got %d", len(got))
				}
				// Both /dev/sda1 and /dev/sdb1 should have the same usage since they share mount point
				for _, du := range got {
					if du.MountPoint == "/" {
						if du.TotalBytes != 1000000 {
							t.Errorf("Expected TotalBytes 1000000 for /, got %d", du.TotalBytes)
						}
					}
					if du.MountPoint == "/data" {
						if du.TotalBytes != 2000000 {
							t.Errorf("Expected TotalBytes 2000000 for /data, got %d", du.TotalBytes)
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.blks.GetDeviceUsages(tt.parts)

			if len(got) != tt.wantCount {
				t.Errorf("GetDeviceUsages() returned %d items, want %d", len(got), tt.wantCount)
			}

			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestFlattenedBlockDevices_GetDeviceUsages_Preserves_Order(t *testing.T) {
	blks := FlattenedBlockDevices{
		{Name: "/dev/sda1", MountPoint: "/"},
		{Name: "/dev/sdb1", MountPoint: "/home"},
		{Name: "/dev/sdc1", MountPoint: "/var"},
	}

	parts := Partitions{
		{
			MountPoint: "/home",
			Usage:      &Usage{TotalBytes: 2000, UsedBytes: 1000, FreeBytes: 1000},
		},
		{
			MountPoint: "/",
			Usage:      &Usage{TotalBytes: 1000, UsedBytes: 500, FreeBytes: 500},
		},
		{
			MountPoint: "/var",
			Usage:      &Usage{TotalBytes: 3000, UsedBytes: 1500, FreeBytes: 1500},
		},
	}

	got := blks.GetDeviceUsages(parts)

	// Verify order is preserved from blks
	if len(got) != 3 {
		t.Fatalf("Expected 3 device usages, got %d", len(got))
	}

	expectedOrder := []string{"/dev/sda1", "/dev/sdb1", "/dev/sdc1"}
	for i, expected := range expectedOrder {
		if got[i].DeviceName != expected {
			t.Errorf("Expected device at index %d to be %s, got %s", i, expected, got[i].DeviceName)
		}
	}
}

func TestFlattenedBlockDevices_GetDeviceUsages_EmptyMountPoint(t *testing.T) {
	blks := FlattenedBlockDevices{
		{Name: "/dev/sda", MountPoint: ""},
		{Name: "/dev/sda1", MountPoint: ""},
		{Name: "/dev/sda2", MountPoint: "/"},
	}

	parts := Partitions{
		{
			MountPoint: "/",
			Usage:      &Usage{TotalBytes: 1000, UsedBytes: 500, FreeBytes: 500},
		},
		{
			MountPoint: "",
			Usage:      &Usage{TotalBytes: 2000, UsedBytes: 1000, FreeBytes: 1000},
		},
	}

	got := blks.GetDeviceUsages(parts)

	// Should only match the one with non-empty mount point
	if len(got) != 1 {
		t.Fatalf("Expected 1 device usage, got %d", len(got))
	}

	if got[0].DeviceName != "/dev/sda2" {
		t.Errorf("Expected DeviceName /dev/sda2, got %s", got[0].DeviceName)
	}
	if got[0].MountPoint != "/" {
		t.Errorf("Expected MountPoint /, got %s", got[0].MountPoint)
	}
}
