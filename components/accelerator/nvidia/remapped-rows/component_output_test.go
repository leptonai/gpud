package remappedrows

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/common"
	query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func TestOutput_isRowRemappingSupported(t *testing.T) {
	tests := []struct {
		name     string
		output   *Output
		expected bool
	}{
		{
			name: "row remapping supported",
			output: &Output{
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
			},
			expected: true,
		},
		{
			name: "row remapping not supported",
			output: &Output{
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: false,
				},
			},
			expected: false,
		},
		{
			name: "empty output",
			output: &Output{
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.output.isRowRemappingSupported()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutput_States(t *testing.T) {
	tests := []struct {
		name    string
		output  *Output
		wantErr bool
	}{
		{
			name: "healthy state",
			output: &Output{
				GPUProductName: "NVIDIA A100",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsNVML: []nvml.RemappedRows{},
			},
			wantErr: false,
		},
		{
			name: "state with suggested actions",
			output: &Output{
				GPUProductName: "NVIDIA A100",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				SuggestedActions: &common.SuggestedActions{
					Descriptions:  []string{"Test action"},
					RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.output.States()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotEmpty(t, states)
			assert.Equal(t, StateNameRemappedRows, states[0].Name)
			assert.Contains(t, states[0].ExtraInfo, StateKeyRemappedRowsData)
			assert.Contains(t, states[0].ExtraInfo, StateKeyRemappedRowsEncoding)
			assert.Equal(t, StateValueRemappedRowsEncodingJSON, states[0].ExtraInfo[StateKeyRemappedRowsEncoding])
		})
	}
}

func TestOutput_Evaluate(t *testing.T) {
	tests := []struct {
		name        string
		output      *Output
		wantReason  string
		wantHealthy bool
		wantErr     bool
	}{
		{
			name:        "nil output",
			output:      nil,
			wantReason:  "no data",
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "row remapping not supported",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: false,
					Message:      "not supported",
				},
			},
			wantReason:  `GPU product name "Test GPU" does not support row remapping (message: "not supported")`,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "healthy state with supported remapping",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
			},
			wantReason:  "no issue detected",
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "NVML GPU needs RMA",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsNVML: []nvml.RemappedRows{
					{
						UUID:                             "GPU-456",
						RemappingFailed:                  true,
						RemappedDueToUncorrectableErrors: 1,
					},
				},
			},
			wantReason:  `GPU GPU-456 qualifies for RMA (remapping failure occurred true, remapped due to uncorrectable errors 1)`,
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "NVML GPU needs reset",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsNVML: []nvml.RemappedRows{
					{
						UUID:             "GPU-456",
						RemappingPending: true,
					},
				},
			},
			wantReason:  `GPU GPU-456 needs reset (pending remapping true)`,
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "multiple issues",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsNVML: []nvml.RemappedRows{
					{
						UUID:             "GPU-456",
						RemappingPending: true,
					},
				},
			},
			wantReason:  `GPU GPU-456 needs reset (pending remapping true)`,
			wantHealthy: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, healthy, err := tt.output.Evaluate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealthy, healthy)
		})
	}
}
