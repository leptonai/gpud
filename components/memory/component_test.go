package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDataGetReason(t *testing.T) {
	// Test with nil data
	var d *Data
	assert.Equal(t, "no memory data", d.getReason())

	// Test with error
	d = &Data{err: assert.AnError}
	assert.Contains(t, d.getReason(), "failed to get memory data")

	// Test with context.DeadlineExceeded error
	d = &Data{err: context.DeadlineExceeded}
	assert.Contains(t, d.getReason(), "check failed with context deadline exceeded")
	assert.Contains(t, d.getReason(), "transient error, please retry")

	// Test with context.Canceled error
	d = &Data{err: context.Canceled}
	assert.Contains(t, d.getReason(), "check failed with context canceled")
	assert.Contains(t, d.getReason(), "transient error, please retry")

	// Test with valid data
	d = &Data{
		TotalBytes: 16 * 1024 * 1024 * 1024,
		UsedBytes:  8 * 1024 * 1024 * 1024,
	}
	assert.Contains(t, d.getReason(), "8.6")
	assert.Contains(t, d.getReason(), "17")
}

func TestDataGetHealth(t *testing.T) {
	// Test with nil data
	var d *Data
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	d = &Data{err: assert.AnError}
	health, healthy = d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with context.DeadlineExceeded error - should be treated as healthy
	d = &Data{err: context.DeadlineExceeded}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy, "Context deadline exceeded should be treated as healthy")

	// Test with context.Canceled error - should be treated as healthy
	d = &Data{err: context.Canceled}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy, "Context canceled should be treated as healthy")

	// Test with valid data
	d = &Data{
		TotalBytes: 16 * 1024 * 1024 * 1024,
		UsedBytes:  8 * 1024 * 1024 * 1024,
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		TotalBytes: 16 * 1024 * 1024 * 1024, // 16GB
		UsedBytes:  8 * 1024 * 1024 * 1024,  // 8GB

		AvailableBytes: 8 * 1024 * 1024 * 1024, // 8GB

		FreeBytes: 7 * 1024 * 1024 * 1024, // 7GB

		VMAllocTotalBytes: 32 * 1024 * 1024 * 1024, // 32GB
		VMAllocUsedBytes:  16 * 1024 * 1024 * 1024, // 16GB

		BPFJITBufferBytes: 1024 * 1024, // 1MB

		ts: time.Now(),
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)

	// Verify extra info
	assert.NotEmpty(t, states[0].ExtraInfo)

	// Try to decode the JSON data
	jsonData := states[0].ExtraInfo["data"]
	assert.NotEmpty(t, jsonData)

	var decodedData Data
	err = json.Unmarshal([]byte(jsonData), &decodedData)
	assert.NoError(t, err)
}

func TestDataGetStatesWithErrors(t *testing.T) {
	// Test with regular error
	t.Run("With regular error", func(t *testing.T) {
		d := &Data{
			err: assert.AnError,
			ts:  time.Now(),
		}
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, "Unhealthy", states[0].Health)
		assert.False(t, states[0].Healthy)
		assert.Contains(t, states[0].Reason, "failed to get memory data")
	})

	// Test with context deadline exceeded error
	t.Run("With context deadline exceeded", func(t *testing.T) {
		d := &Data{
			err: context.DeadlineExceeded,
			ts:  time.Now(),
		}
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, "Healthy", states[0].Health)
		assert.True(t, states[0].Healthy)
		assert.Contains(t, states[0].Reason, "check failed with context deadline exceeded")
		assert.Contains(t, states[0].Reason, "transient error, please retry")
	})

	// Test with context canceled error
	t.Run("With context canceled", func(t *testing.T) {
		d := &Data{
			err: context.Canceled,
			ts:  time.Now(),
		}
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, "Healthy", states[0].Health)
		assert.True(t, states[0].Healthy)
		assert.Contains(t, states[0].Reason, "check failed with context canceled")
		assert.Contains(t, states[0].Reason, "transient error, please retry")
	})
}

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}
