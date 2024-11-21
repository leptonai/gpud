package infiniband

import (
	"testing"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	"github.com/leptonai/gpud/components/common"
)

func TestOutputStates(t *testing.T) {
	tests := []struct {
		name            string
		cfg             Config
		o               *Output
		expectedHealthy bool
		expectedReason  string
	}{
		{
			name: "GTX 4090 state",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{
					PortCount: 1,
					Rate:      400,
				},
			},
			o: &Output{
				GPUProductName: "NVIDIA GeForce RTX 4090",
			},
			expectedHealthy: true,
			expectedReason:  `"NVIDIA GeForce RTX 4090" GPUs do not support infiniband`,
		},
		{
			name: "Healthy state",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{
					PortCount: 1,
					Rate:      400,
				},
			},
			o: &Output{
				GPUProductName:        "NVIDIA A100",
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat:                infiniband.IbstatOutput{},
			},
			expectedHealthy: true,
			expectedReason:  "no infiniband class found or no ibstat exists or no ibstat error found",
		},
		{
			name: "Unhealthy state",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{
					PortCount: 0,
					Rate:      400,
				},
			},
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat: infiniband.IbstatOutput{
					Errors: []string{"Error 1", "Error 2"},
				},
			},
			expectedHealthy: false,
			expectedReason:  "infiniband suppported but ibstat errors found: Error 1, Error 2",
		},
		{
			name: "No ibstat state",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{
					PortCount: 0,
					Rate:      400,
				},
			},
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				InfinibandClassExists: false,
				IbstatExists:          false,
			},
			expectedHealthy: true,
			expectedReason:  "no infiniband class found or no ibstat exists or no ibstat error found",
		},
		{
			name: "Not all cards active and up (A100) with default rate",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{},
			},
			o: &Output{
				GPUProductName:        "NVIDIA A100",
				GPUCount:              8,
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat: infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200}},
					},
				},
			},
			expectedHealthy: false,
			expectedReason:  "only 6 out of 8 ibstat cards are active and link up (expected rate: 200 Gb/sec)",
		},
		{
			name: "Not all cards active and up (H100) with default rate",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{},
			},
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				GPUCount:              8,
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat: infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 400}},
					},
				},
			},
			expectedHealthy: false,
			expectedReason:  "only 6 out of 8 ibstat cards are active and link up (expected rate: 400 Gb/sec)",
		},
		{
			name: "Not all cards active and up (H100) with lower rate",
			cfg: Config{
				ExpectedPortStates: ExpectedPortStates{
					PortCount: 6,
					Rate:      200,
				},
			},
			o: &Output{
				GPUProductName:        "NVIDIA H100",
				GPUCount:              8,
				InfinibandClassExists: true,
				IbstatExists:          true,
				Ibstat: infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
						{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200}},
					},
				},
			},
			expectedHealthy: true,
			expectedReason:  "no infiniband class found or no ibstat exists or no ibstat error found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.o.States(tt.cfg)
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
					if len(state.SuggestedActions.RepairActions) != 1 || state.SuggestedActions.RepairActions[0] != common.RepairActionTypeHardwareInspection {
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
