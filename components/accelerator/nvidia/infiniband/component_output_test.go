package infiniband

import (
	"testing"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/common"
)

func TestOutputStates(t *testing.T) {
	tests := []struct {
		name            string
		o               *Output
		expectedHealthy bool
		expectedReason  string
	}{
		{
			name: "GTX 4090 state",
			o: &Output{
				GPUProductName: "NVIDIA GeForce RTX 4090",
			},
			expectedHealthy: true,
			expectedReason:  `"NVIDIA GeForce RTX 4090" GPUs do not support infiniband`,
		},
		{
			name: "Healthy state",
			o: &Output{
				GPUProductName:        "NVIDIA A100",
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat:                nvidia_query.IbstatOutput{},
			},
			expectedHealthy: true,
			expectedReason:  "no infiniband class found or no ibstat exists or no ibstat error found",
		},
		{
			name: "Unhealthy state",
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat: nvidia_query.IbstatOutput{
					Errors: []string{"Error 1", "Error 2"},
				},
			},
			expectedHealthy: false,
			expectedReason:  "infiniband suppported but ibstat errors found: Error 1, Error 2",
		},
		{
			name: "No ibstat state",
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				InfinibandClassExists: false,
				IbstatExists:          false,
			},
			expectedHealthy: true,
			expectedReason:  "no infiniband class found or no ibstat exists or no ibstat error found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.o.States()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(states) != 1 {
				t.Fatalf("expected 1 state, got %d", len(states))
			}

			state := states[0]
			if state.Healthy != tt.expectedHealthy {
				t.Errorf("expected Healthy to be %v, got %v", tt.expectedHealthy, state.Healthy)
			}

			if state.Reason != tt.expectedReason {
				t.Errorf("expected Reason to be %s, got %s", tt.expectedReason, state.Reason)
			}

			// Additional checks for ExtraInfo and SuggestedActions
			if !tt.o.IbstatExists {
				if state.ExtraInfo[nvidia_query.StateKeyIbstatExists] != "false" {
					t.Errorf("expected IbstatExists to be false, got %s", state.ExtraInfo[nvidia_query.StateKeyIbstatExists])
				}
			} else {
				if state.ExtraInfo[nvidia_query.StateKeyIbstatExists] != "true" {
					t.Errorf("expected IbstatExists to be true, got %s", state.ExtraInfo[nvidia_query.StateKeyIbstatExists])
				}
			}

			if !tt.expectedHealthy {
				if state.SuggestedActions == nil {
					t.Error("expected SuggestedActions to be non-nil for unhealthy state")
				} else {
					if len(state.SuggestedActions.RepairActions) != 1 || state.SuggestedActions.RepairActions[0] != common.RepairActionTypeRepairHardware {
						t.Errorf("expected RepairActions to be [RepairHardware], got %v", state.SuggestedActions.RepairActions)
					}
					if len(state.SuggestedActions.Descriptions) != 1 || state.SuggestedActions.Descriptions[0] != "potential infiniband switch/hardware issue needs immediate attention" {
						t.Errorf("unexpected SuggestedActions description: %v", state.SuggestedActions.Descriptions)
					}
				}
			} else {
				if state.SuggestedActions != nil {
					t.Error("expected SuggestedActions to be nil for healthy state")
				}
			}
		})
	}
}
