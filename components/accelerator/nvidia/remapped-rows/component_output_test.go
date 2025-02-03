package remappedrows

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
)

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
			name: "GPU without row remapping support",
			output: &Output{
				GPUProductName: "NVIDIA T4",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: false,
					Message:      "Row remapping not supported",
				},
			},
			wantReason:  `GPU product name "NVIDIA T4" does not support row remapping (message: "Row remapping not supported")`,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "SMI GPU qualifies for RMA",
			output: &Output{
				GPUProductName: "NVIDIA A100",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsSMI: []query.ParsedSMIRemappedRows{
					{
						ID:                               "GPU-123",
						RemappingFailed:                  "Yes",
						RemappedDueToUncorrectableErrors: "1",
						RemappingPending:                 "No",
					},
				},
			},
			wantReason:  "nvidia-smi GPU GPU-123 qualifies for RMA (remapping failure occurred Yes, remapped due to uncorrectable errors 1)",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "SMI GPU needs reset",
			output: &Output{
				GPUProductName: "NVIDIA A100",
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
			wantReason:  "nvidia-smi GPU GPU-123 needs reset (pending remapping true)",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "NVML GPU qualifies for RMA",
			output: &Output{
				GPUProductName: "NVIDIA A100",
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
			wantReason:  "nvml GPU GPU-456 qualifies for RMA (remapping failure occurred true, remapped due to uncorrectable errors 1)",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "NVML GPU needs reset",
			output: &Output{
				GPUProductName: "NVIDIA A100",
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
			wantReason:  "nvml GPU GPU-456 needs reset (pending remapping true)",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "Multiple issues",
			output: &Output{
				GPUProductName: "NVIDIA A100",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsSMI: []query.ParsedSMIRemappedRows{
					{
						ID:                               "GPU-123",
						RemappingFailed:                  "Yes",
						RemappedDueToUncorrectableErrors: "1",
						RemappingPending:                 "No",
					},
				},
				RemappedRowsNVML: []nvml.RemappedRows{
					{
						UUID:             "GPU-456",
						RemappingPending: true,
					},
				},
			},
			wantReason:  "nvidia-smi GPU GPU-123 qualifies for RMA (remapping failure occurred Yes, remapped due to uncorrectable errors 1), nvml GPU GPU-456 needs reset (pending remapping true)",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "No issues detected",
			output: &Output{
				GPUProductName: "NVIDIA A100",
				MemoryErrorManagementCapabilities: query.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRowsSMI:  []query.ParsedSMIRemappedRows{},
				RemappedRowsNVML: []nvml.RemappedRows{},
			},
			wantReason:  "no issue detected",
			wantHealthy: true,
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
			assert.Equal(t, tt.wantHealthy, healthy)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}
