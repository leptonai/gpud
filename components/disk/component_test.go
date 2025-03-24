package disk

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	mountPoints := []string{"/data"}
	mountTargets := []string{"/target"}

	comp := New(ctx, mountPoints, mountTargets)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestComponentLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := New(ctx, []string{}, []string{})

	// Test Start
	err := comp.Start()
	assert.NoError(t, err)

	// Give some time for the goroutine to start
	time.Sleep(100 * time.Millisecond)

	// Test States before any data
	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "no disk data", states[0].Reason)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)

	// Test Events
	events, err := comp.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)

	// Test Close
	err = comp.Close()
	assert.NoError(t, err)
}

func TestDataStates(t *testing.T) {
	// Test nil data
	var nilData *Data
	states, err := nilData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "no disk data", states[0].Reason)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)

	// Test data with error
	errData := &Data{
		err: assert.AnError,
	}
	states, err = errData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Reason, "failed to get disk data")
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)

	// Test healthy data
	healthyData := &Data{
		ExtPartitions: make([]disk.Partition, 2),
		BlockDevices:  make([]disk.BlockDevice, 3),
	}
	states, err = healthyData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Reason, "found 2 ext4 partitions and 3 block devices")
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
}

func TestDataHealth(t *testing.T) {
	// Test nil data
	var nilData *Data
	health, healthy := nilData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test data with error
	errData := &Data{
		err: assert.AnError,
	}
	health, healthy = errData.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test healthy data
	healthyData := &Data{}
	health, healthy = healthyData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)
}

func TestDataReason(t *testing.T) {
	// Test nil data
	var nilData *Data
	reason := nilData.getReason()
	assert.Equal(t, "no disk data", reason)

	// Test data with error
	errData := &Data{
		err: assert.AnError,
	}
	reason = errData.getReason()
	assert.Contains(t, reason, "failed to get disk data")

	// Test healthy data
	healthyData := &Data{
		ExtPartitions: make([]disk.Partition, 2),
		BlockDevices:  make([]disk.BlockDevice, 3),
	}
	reason = healthyData.getReason()
	assert.Contains(t, reason, "found 2 ext4 partitions and 3 block devices")
}
