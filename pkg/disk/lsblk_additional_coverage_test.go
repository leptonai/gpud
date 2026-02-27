package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBlockDevicesWithLsblk_DependencyBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("fallback parser when version parsing fails", func(t *testing.T) {
		devs, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "invalid version", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return []byte(`{"blockdevices":[{"name":"/dev/sda","type":"disk","mountpoint":"/","fstype":"ext4"}]}`), nil
			},
			findMnt: FindMnt,
		})
		require.NoError(t, err)
		require.Len(t, devs, 1)
		assert.Equal(t, "/dev/sda", devs[0].Name)
	})

	t.Run("json parser path", func(t *testing.T) {
		devs, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "lsblk from util-linux 2.37.0", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return []byte(`{"blockdevices":[{"name":"/dev/sdb","type":"disk","mountpoint":"/data","fstype":"xfs"}]}`), nil
			},
			findMnt: FindMnt,
		})
		require.NoError(t, err)
		require.Len(t, devs, 1)
		assert.Equal(t, "/dev/sdb", devs[0].Name)
	})

	t.Run("pairs parser path", func(t *testing.T) {
		devs, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "lsblk from util-linux 2.36.0", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return []byte(`NAME="/dev/sda" TYPE="disk" MOUNTPOINT="" FSTYPE="" PKNAME=""
NAME="/dev/sda1" TYPE="part" MOUNTPOINT="/boot" FSTYPE="ext4" PKNAME="/dev/sda"`), nil
			},
			findMnt: FindMnt,
		})
		require.NoError(t, err)
		require.Len(t, devs, 1)
		require.Len(t, devs[0].Children, 1)
		assert.Equal(t, "/dev/sda1", devs[0].Children[0].Name)
	})

	t.Run("parse error is propagated", func(t *testing.T) {
		_, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "lsblk from util-linux 2.37.0", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return []byte(`{"blockdevices":[invalid]}`), nil
			},
			findMnt: FindMnt,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})
}

func TestParseLsblkPairsWithFindMnt_EdgeCases(t *testing.T) {
	ctx := context.Background()
	findMnt := func(context.Context, string) (*FindMntOutput, error) {
		return nil, nil
	}

	t.Run("empty input", func(t *testing.T) {
		_, err := parseLsblkPairsWithFindMnt(ctx, nil, findMnt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty input provided")
	})

	t.Run("hierarchy build failure", func(t *testing.T) {
		_, err := parseLsblkPairsWithFindMnt(
			ctx,
			[]byte(`NAME="/dev/sda1" TYPE="part" MOUNTPOINT="/boot" FSTYPE="ext4" PKNAME="/dev/sda"`),
			findMnt,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build disk hierarchy failed")
	})
}

func TestWrapperHelpers_BasicCoverage(t *testing.T) {
	ctx := context.Background()

	// Wrapper should no-op for empty mountpoint and should not panic.
	dev := &BlockDevice{Name: "/dev/sda1", MountPoint: "", FSType: ""}
	fillFstypeFromFindmnt(ctx, dev, map[string]string{})
	assert.Equal(t, "", dev.FSType)

	// toCustomBool should handle both valid and invalid inputs.
	assert.True(t, toCustomBool("true").Bool)
	assert.False(t, toCustomBool("not-a-bool").Bool)

	var cb CustomBool
	err := cb.UnmarshalJSON([]byte(`"invalid"`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown value")
}

func TestDecideLsblkFlag_LargeVersionStillParses(t *testing.T) {
	ctx := context.Background()

	// Extremely large version should still select the JSON path.
	flags, parser, err := decideLsblkFlag(ctx, "lsblk from util-linux 999999999999999999999999999999999999.1")
	require.NoError(t, err)
	assert.Contains(t, flags, "--json")
	assert.NotNil(t, parser)
}
