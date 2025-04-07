package disk

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	assert.Equal(t, Name, c.Name())
}

func TestNewComponent(t *testing.T) {
	ctx := context.Background()
	mountPoints := []string{"/mnt/test1"}
	mountTargets := []string{"/mnt/test2"}

	c := New(ctx, mountPoints, mountTargets).(*component)
	defer c.Close()

	// Check if mount points are correctly added to the tracking map
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test1")
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test2")
}

func TestComponentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	err := c.Start()
	assert.NoError(t, err)
}

func TestComponentClose(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)

	err := c.Close()
	assert.NoError(t, err)
}

func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestEmptyDataStates(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	// No data set yet
	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Equal(t, "no disk data", states[0].Reason)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
}

func TestDataWithError(t *testing.T) {
	d := &Data{
		err: assert.AnError,
	}

	reason := d.getReason()
	assert.Contains(t, reason, "failed to get disk data")

	health, healthy := d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)
}

func TestDataWithPartitionsAndDevices(t *testing.T) {
	d := &Data{
		ExtPartitions: disk.Partitions{
			{Device: "/dev/sda1", MountPoint: "/mnt/data1"},
			{Device: "/dev/sda2", MountPoint: "/mnt/data2"},
		},
		BlockDevices: disk.BlockDevices{
			{Name: "sda", Type: "disk"},
			{Name: "sdb", Type: "disk"},
			{Name: "sdc", Type: "disk"},
		},
	}

	reason := d.getReason()
	assert.Equal(t, "found 2 ext4 partitions and 3 block devices", reason)

	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		ExtPartitions: disk.Partitions{
			{Device: "/dev/sda1", MountPoint: "/mnt/data1"},
		},
		BlockDevices: disk.BlockDevices{
			{Name: "sda", Type: "disk"},
		},
	}

	states, err := d.getStates()
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "found 1 ext4 partitions and 1 block devices", states[0].Reason)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Contains(t, states[0].ExtraInfo, "data")
	assert.Contains(t, states[0].ExtraInfo, "encoding")
}

func TestDataGetError(t *testing.T) {
	// Test with error
	d := &Data{
		err: assert.AnError,
	}
	errStr := d.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test without error
	d = &Data{}
	errStr = d.getError()
	assert.Empty(t, errStr)

	// Test with nil data
	var nilData *Data
	errStr = nilData.getError()
	assert.Empty(t, errStr)
}

func TestDataGetStatesWithError(t *testing.T) {
	d := &Data{
		err: assert.AnError,
	}

	states, err := d.getStates()
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Reason, "failed to get disk data")
	assert.Equal(t, "Unhealthy", states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
	assert.Contains(t, states[0].ExtraInfo, "data")
	assert.Contains(t, states[0].ExtraInfo, "encoding")
}

func TestComponentStatesWithError(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	// Manually set lastData with an error
	c.lastMu.Lock()
	c.lastData = &Data{
		err: assert.AnError,
	}
	c.lastMu.Unlock()

	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Reason, "failed to get disk data")
	assert.Equal(t, "Unhealthy", states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}
