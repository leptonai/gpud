package hwslowdown

import (
	"encoding/json"
	"strings"
	"testing"

	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
)

func TestOutput_States(t *testing.T) {
	tests := []struct {
		name          string
		output        Output
		wantHealthy   bool
		wantErrSubstr []string
		wantNoErrMsg  string
		wantErr       bool
	}{
		{
			name: "empty output should be healthy",
			output: Output{
				HWSlowdownEventsNVML: []nvidia_query_nvml.ClockEvents{
					{
						UUID:    "gpu-123",
						Reasons: []string{"non-critical reason"},
					},
				},
			},
			wantHealthy:  true,
			wantNoErrMsg: "no hardware slowdown found in nvidia-smi",
		},
		{
			name: "output with NVML reasons",
			output: Output{
				HWSlowdownEventsNVML: []nvidia_query_nvml.ClockEvents{
					{
						UUID:              "gpu-123",
						HWSlowdownReasons: []string{"test reason"},
					},
				},
			},
			wantHealthy:  true,
			wantNoErrMsg: "no hardware slowdown found in nvidia-smi",
		},
		{
			name: "output with HW slowdown flags",
			output: Output{
				HWSlowdownSMI: HWSlowdownSMI{
					Errors: []string{
						"gpu-123 hw slowdown (smi)",
						"gpu-123 hw slowdown thermal (smi)",
						"gpu-123 hw slowdown power brake (smi)",
					},
				},
			},
			wantHealthy: false,
			wantErrSubstr: []string{
				"hw slowdown found in nvidia-smi: gpu-123 hw slowdown (smi), gpu-123 hw slowdown thermal (smi), gpu-123 hw slowdown power brake (smi)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.output.States()

			if (err != nil) != tt.wantErr {
				t.Errorf("Output.States() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if len(states) != 1 {
				t.Errorf("Output.States() returned %d states, want 1", len(states))
				return
			}

			state := states[0]

			if state.Name != StateNameHWSlowdown {
				t.Errorf("State.Name = %v, want %v", state.Name, StateNameHWSlowdown)
			}

			if state.Healthy != tt.wantHealthy {
				t.Errorf("State.Healthy = %v, want %v", state.Healthy, tt.wantHealthy)
			}

			if tt.wantHealthy {
				if state.Reason != tt.wantNoErrMsg {
					t.Errorf("State.Reason = %v, want %v", state.Reason, tt.wantNoErrMsg)
				}
			} else {
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(state.Reason, substr) {
						t.Errorf("State.Reason = %v, want it to contain %v", state.Reason, substr)
					}
				}
			}

			// Verify ExtraInfo
			if state.ExtraInfo[StateKeyHWSlowdownEncoding] != StateValueHWSlowdownEncodingJSON {
				t.Errorf("ExtraInfo encoding = %v, want %v",
					state.ExtraInfo[StateKeyHWSlowdownEncoding],
					StateValueHWSlowdownEncodingJSON)
			}

			// Verify the JSON data can be parsed back
			var parsedOutput Output
			if err := json.Unmarshal([]byte(state.ExtraInfo[StateKeyHWSlowdownData]), &parsedOutput); err != nil {
				t.Errorf("Failed to parse ExtraInfo data: %v", err)
			}
		})
	}
}
