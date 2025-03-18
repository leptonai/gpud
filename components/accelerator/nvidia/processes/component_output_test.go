package processes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func TestData_States(t *testing.T) {
	t.Run("with processes", func(t *testing.T) {
		// Create test data with a single process
		d := &Data{
			Processes: []nvidia_query_nvml.Processes{
				{
					UUID: "GPU-12345678",
					RunningProcesses: []nvidia_query_nvml.Process{
						{
							PID:                123,
							CmdArgs:            []string{"/usr/bin/test", "-arg1"},
							GPUUsedPercent:     50,
							GPUUsedMemoryBytes: 1024 * 1024 * 100,
						},
					},
				},
			},
		}

		// Call States method
		states, err := d.States()

		// Verify results
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, "processes", states[0].Name)
		assert.True(t, states[0].Healthy)
		assert.Equal(t, "total 1 processes", states[0].Reason)

		// Validate that data field contains correct JSON
		dataJSON := states[0].ExtraInfo["data"]
		assert.NotEmpty(t, dataJSON)
		assert.Equal(t, "json", states[0].ExtraInfo["encoding"])

		// Unmarshal and verify the data matches our original struct
		var parsedData Data
		err = json.Unmarshal([]byte(dataJSON), &parsedData)
		assert.NoError(t, err)
		assert.Len(t, parsedData.Processes, 1)
		assert.Equal(t, "GPU-12345678", parsedData.Processes[0].UUID)
	})

	t.Run("with empty processes", func(t *testing.T) {
		// Create test data with no processes
		d := &Data{
			Processes: []nvidia_query_nvml.Processes{},
		}

		// Call States method
		states, err := d.States()

		// Verify results
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, "processes", states[0].Name)
		assert.True(t, states[0].Healthy)
		assert.Equal(t, "total 0 processes", states[0].Reason)
		assert.NotEmpty(t, states[0].ExtraInfo["data"])
	})

	t.Run("with nil data", func(t *testing.T) {
		// Test with nil Data pointer
		var d *Data = nil

		// Call States method
		states, err := d.States()

		// Verify results
		assert.NoError(t, err)
		assert.Len(t, states, 0)
	})
}
