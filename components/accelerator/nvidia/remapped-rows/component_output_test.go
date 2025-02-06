package remappedrows

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	"github.com/leptonai/gpud/components/common"
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

func TestParseOutputJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "valid JSON",
			input: `{
				"gpu_product_name": "NVIDIA A100",
				"memory_error_management_capabilities": {
					"row_remapping": true
				},
				"remapped_rows_smi": [],
				"remapped_rows_nvml": []
			}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ParseOutputJSON([]byte(tt.input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, output)
		})
	}
}

func TestParseStateRemappedRows(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]string
		wantErr bool
	}{
		{
			name: "valid state",
			input: map[string]string{
				StateKeyRemappedRowsData: `{
					"gpu_product_name": "NVIDIA A100",
					"memory_error_management_capabilities": {
						"row_remapping": true
					},
					"remapped_rows_smi": [],
					"remapped_rows_nvml": []
				}`,
			},
			wantErr: false,
		},
		{
			name: "invalid JSON in state",
			input: map[string]string{
				StateKeyRemappedRowsData: `{invalid json}`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ParseStateRemappedRows(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, output)
		})
	}
}

func TestParseStatesToOutput(t *testing.T) {
	tests := []struct {
		name    string
		states  []components.State
		wantErr bool
	}{
		{
			name: "valid remapped rows state",
			states: []components.State{
				{
					Name: StateNameRemappedRows,
					ExtraInfo: map[string]string{
						StateKeyRemappedRowsData: `{
							"gpu_product_name": "NVIDIA A100",
							"memory_error_management_capabilities": {
								"row_remapping": true
							},
							"remapped_rows_smi": [],
							"remapped_rows_nvml": []
						}`,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unknown state name",
			states: []components.State{
				{
					Name: "unknown_state",
				},
			},
			wantErr: true,
		},
		{
			name:    "no states",
			states:  []components.State{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ParseStatesToOutput(tt.states...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, output)
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
				RemappedRowsSMI:  []query.ParsedSMIRemappedRows{},
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
			name: "SMI GPU needs RMA",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsSMI: []query.ParsedSMIRemappedRows{
					{
						ID:                               "GPU-123",
						RemappingFailed:                  "Yes",
						RemappingPending:                 "No",
						RemappedDueToUncorrectableErrors: "5",
					},
				},
			},
			wantReason:  `nvidia-smi GPU GPU-123 qualifies for RMA (remapping failure occurred Yes, remapped due to uncorrectable errors 5)`,
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "SMI GPU needs reset",
			output: &Output{
				GPUProductName: "Test GPU",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsSMI: []query.ParsedSMIRemappedRows{
					{
						ID:                               "GPU-123",
						RemappingPending:                 "Yes",
						RemappingFailed:                  "No",
						RemappedDueToUncorrectableErrors: "0",
					},
				},
			},
			wantReason:  `nvidia-smi GPU GPU-123 needs reset (pending remapping true)`,
			wantHealthy: false,
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
			wantReason:  `nvml GPU GPU-456 qualifies for RMA (remapping failure occurred true, remapped due to uncorrectable errors 1)`,
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
			wantReason:  `nvml GPU GPU-456 needs reset (pending remapping true)`,
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
				RemappedRowsSMI: []query.ParsedSMIRemappedRows{
					{
						ID:                               "GPU-123",
						RemappingFailed:                  "Yes",
						RemappingPending:                 "No",
						RemappedDueToUncorrectableErrors: "5",
					},
				},
			},
			wantReason:  `nvidia-smi GPU GPU-123 qualifies for RMA (remapping failure occurred Yes, remapped due to uncorrectable errors 5), nvml GPU GPU-456 needs reset (pending remapping true)`,
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
