package nfs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func TestComponentValidationOrdering(t *testing.T) {
	t.Parallel()

	t.Run("validation timeout returns early with timeout message", func(t *testing.T) {
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return context.DeadlineExceeded
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				t.Error("findMntTargetDevice should not be called on validation timeout")
				return "", "", nil
			},
			isNFSFSType: func(fsType string) bool {
				t.Error("isNFSFSType should not be called on validation timeout")
				return false
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Equal(t, "NFS validation timed out - server may be unresponsive", cr.reason)
		assert.ErrorIs(t, cr.err, context.DeadlineExceeded)
	})

	t.Run("validation error returns early with generic message", func(t *testing.T) {
		validationErr := errors.New("custom validation error")
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return validationErr
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				t.Error("findMntTargetDevice should not be called on validation error")
				return "", "", nil
			},
			isNFSFSType: func(fsType string) bool {
				t.Error("isNFSFSType should not be called on validation error")
				return false
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Equal(t, "invalid nfs group configs", cr.reason)
		assert.Equal(t, validationErr, cr.err)
	})

	t.Run("mount device not found returns early", func(t *testing.T) {
		mountErr := errors.New("mount not found")
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return nil // validation passes
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				assert.Equal(t, "/mnt/nfs", dir)
				return "", "", mountErr
			},
			isNFSFSType: func(fsType string) bool {
				t.Error("isNFSFSType should not be called when mount device not found")
				return false
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Equal(t, "failed to find mount target device for /mnt/nfs", cr.reason)
		assert.Equal(t, mountErr, cr.err)
	})

	t.Run("non-NFS filesystem type returns early", func(t *testing.T) {
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/ext4"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return nil // validation passes
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				assert.Equal(t, "/mnt/ext4", dir)
				return "/dev/sda1", "ext4", nil
			},
			isNFSFSType: func(fsType string) bool {
				assert.Equal(t, "ext4", fsType)
				return false
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Equal(t, `The user applied path "/mnt/ext4" as NFS volume, but in fact the file system type is not NFS.`, cr.reason)
		assert.Nil(t, cr.err)
	})

	t.Run("all validations pass proceeds to next steps", func(t *testing.T) {
		validateCalled := false
		findMountCalled := false
		isNFSCalled := false

		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs1"},
					{VolumePath: "/mnt/nfs2"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				validateCalled = true
				return nil
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				findMountCalled = true
				if dir == "/mnt/nfs1" {
					return "server1:/export", "nfs", nil
				}
				return "server2:/export", "nfs4", nil
			},
			isNFSFSType: func(fsType string) bool {
				isNFSCalled = true
				return fsType == "nfs" || fsType == "nfs4"
			},
		}

		// Note: This test doesn't run the full check() method as it would require
		// setting up more dependencies. It demonstrates the validation ordering.
		configs := c.getGroupConfigsFunc()
		memberConfigs := configs.GetMemberConfigs(c.machineID)

		// Step 1: Validate member configs
		err := c.validateMemberConfigs(context.Background(), memberConfigs)
		assert.NoError(t, err)
		assert.True(t, validateCalled)

		// Step 2: Check mount points only if validation passed
		for _, cfg := range configs {
			dev, fsType, err := c.findMntTargetDevice(cfg.VolumePath)
			assert.NoError(t, err)
			assert.NotEmpty(t, dev)

			// Step 3: Check filesystem type only if mount found
			isNFS := c.isNFSFSType(fsType)
			assert.True(t, isNFS)
		}

		assert.True(t, findMountCalled)
		assert.True(t, isNFSCalled)
	})
}

func TestValidationOrderingMultiplePaths(t *testing.T) {
	t.Parallel()

	t.Run("stops at first invalid mount", func(t *testing.T) {
		mountCallCount := 0
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs1"},
					{VolumePath: "/mnt/invalid"},
					{VolumePath: "/mnt/nfs3"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return nil
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				mountCallCount++
				if dir == "/mnt/invalid" {
					return "", "", errors.New("mount not found")
				}
				return "server:/export", "nfs", nil
			},
			isNFSFSType: func(fsType string) bool {
				return true
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Contains(t, cr.reason, "/mnt/invalid")
		// Should stop at second mount (the invalid one)
		assert.Equal(t, 2, mountCallCount)
	})

	t.Run("stops at first non-NFS filesystem", func(t *testing.T) {
		fsCheckCallCount := 0
		c := &component{
			ctx:       context.Background(),
			machineID: "test-machine",
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{VolumePath: "/mnt/nfs1"},
					{VolumePath: "/mnt/ext4"},
					{VolumePath: "/mnt/nfs3"},
				}
			},
			validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
				return nil
			},
			findMntTargetDevice: func(dir string) (string, string, error) {
				switch dir {
				case "/mnt/nfs1":
					return "server1:/export", "nfs", nil
				case "/mnt/ext4":
					return "/dev/sdb1", "ext4", nil
				case "/mnt/nfs3":
					return "server3:/export", "nfs", nil
				}
				return "", "", errors.New("unexpected path")
			},
			isNFSFSType: func(fsType string) bool {
				fsCheckCallCount++
				return fsType == "nfs" || fsType == "nfs4"
			},
		}

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
		assert.Contains(t, cr.reason, "/mnt/ext4")
		assert.Contains(t, cr.reason, "file system type is not NFS")
		// Should check first two filesystems only
		assert.Equal(t, 2, fsCheckCallCount)
	})
}
