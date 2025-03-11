package memory

import (
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

	// Test with valid data
	d = &Data{
		TotalBytes:     16 * 1024 * 1024 * 1024, // 16GB
		TotalHumanized: "16.00 GB",
		UsedBytes:      8 * 1024 * 1024 * 1024, // 8GB
		UsedHumanized:  "8.00 GB",
		UsedPercent:    "50.00",
	}
	assert.Contains(t, d.getReason(), "8.00 GB")
	assert.Contains(t, d.getReason(), "16.00 GB")
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

	// Test with valid data - normal usage
	d = &Data{
		UsedPercent: "50.00",
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with valid data - high usage
	d = &Data{
		UsedPercent: "95.00",
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health) // Memory usage alone doesn't trigger unhealthy state
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		TotalBytes:     16 * 1024 * 1024 * 1024, // 16GB
		TotalHumanized: "16.00 GB",
		UsedBytes:      8 * 1024 * 1024 * 1024, // 8GB
		UsedHumanized:  "8.00 GB",
		UsedPercent:    "50.00",

		AvailableBytes:     8 * 1024 * 1024 * 1024, // 8GB
		AvailableHumanized: "8.00 GB",

		FreeBytes:     7 * 1024 * 1024 * 1024, // 7GB
		FreeHumanized: "7.00 GB",

		VMAllocTotalBytes:     32 * 1024 * 1024 * 1024, // 32GB
		VMAllocTotalHumanized: "32.00 GB",
		VMAllocUsedBytes:      16 * 1024 * 1024 * 1024, // 16GB
		VMAllocUsedHumanized:  "16.00 GB",
		VMAllocUsedPercent:    "50.00",

		BPFJITBufferBytes:     1024 * 1024, // 1MB
		BPFJITBufferHumanized: "1.00 MB",

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
	assert.Equal(t, "50.00", decodedData.UsedPercent)
	assert.Equal(t, "16.00 GB", decodedData.TotalHumanized)
}
