package disk

import (
	"context"
	"fmt"
	"testing"
)

func TestParseLsblkJSONWithNullFstype(t *testing.T) {
	testJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "size": 137438953472,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": "Msft    ",
         "model": "Virtual Disk    ",
         "rev": "1.0 ",
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/sda1",
               "type": "part",
               "size": 137322544640,
               "rota": true,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0",
               "fstype": null,
               "fsused": "97563111424",
               "partuuid": null
            }
         ]
      }
   ]
}`

	findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
		if target == "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0" {
			return &FindMntOutput{
				Filesystems: []FoundMnt{
					{Fstype: "ext4"},
				},
			}, nil
		}
		return nil, fmt.Errorf("findmnt mock: unexpected target %s", target)
	}

	ctx := context.Background()
	devs, err := parseLsblkJSONWithFindMnt(ctx, []byte(testJSON), findMnt)
	if err != nil {
		t.Fatalf("Failed to parse lsblk JSON: %v", err)
	}

	if len(devs) == 0 {
		t.Fatal("Expected at least one device, got none")
	}

	// Check that the parent device was parsed
	if devs[0].Name != "/dev/sda" {
		t.Errorf("Expected parent device name /dev/sda, got %s", devs[0].Name)
	}

	// Check that the child device was parsed
	if len(devs[0].Children) != 1 {
		t.Fatalf("Expected 1 child device, got %d", len(devs[0].Children))
	}

	child := devs[0].Children[0]
	if child.Name != "/dev/sda1" {
		t.Errorf("Expected child device name /dev/sda1, got %s", child.Name)
	}

	// The mountpoint should be preserved
	expectedMountPoint := "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0"
	if child.MountPoint != expectedMountPoint {
		t.Errorf("Expected mount point %s, got %s", expectedMountPoint, child.MountPoint)
	}

	// Assert that fstype was filled by findmnt fallback
	if child.FSType != "ext4" {
		t.Errorf("Expected fstype to be 'ext4' from findmnt fallback, got '%s'", child.FSType)
	}

	t.Logf("Successfully parsed lsblk output with null fstype values and fallback to findmnt")
	t.Logf("Parent device: %s, Type: %s", devs[0].Name, devs[0].Type)
	t.Logf("Child device: %s, Type: %s, MountPoint: %s, FSType: %s", child.Name, child.Type, child.MountPoint, child.FSType)
}

func TestGetFstypeFromFindmnt(t *testing.T) {
	// This test verifies the helper function exists and handles edge cases
	ctx := context.Background()

	// Test with empty mount point
	fstype := getFstypeFromFindmnt(ctx, "")
	if fstype != "" {
		t.Errorf("Expected empty string for empty mount point, got %s", fstype)
	}

	// Note: Testing with a real mount point would require mocking or a real filesystem
	// For unit testing, we're verifying the function exists and handles basic cases
	t.Log("getFstypeFromFindmnt function is available for fallback")
}
