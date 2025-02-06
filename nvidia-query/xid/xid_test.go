package xid

import (
	"testing"

	"github.com/leptonai/gpud/common"
)

func TestDetail_IsOnlyHWError(t *testing.T) {
	tests := []struct {
		name string
		d    Detail
		want bool
	}{
		{
			name: "only hardware error",
			d: Detail{
				PotentialHWError:                true,
				PotentialDriverError:            false,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: true,
		},
		{
			name: "hardware error with driver error",
			d: Detail{
				PotentialHWError:                true,
				PotentialDriverError:            true,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
		{
			name: "no hardware error",
			d: Detail{
				PotentialHWError:                false,
				PotentialDriverError:            true,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.IsOnlyHWError(); got != tt.want {
				t.Errorf("Detail.IsOnlyHWError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetail_IsOnlyUserAppError(t *testing.T) {
	tests := []struct {
		name string
		d    Detail
		want bool
	}{
		{
			name: "only user app error",
			d: Detail{
				PotentialHWError:                false,
				PotentialDriverError:            false,
				PotentialUserAppError:           true,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: true,
		},
		{
			name: "user app error with driver error",
			d: Detail{
				PotentialHWError:                false,
				PotentialDriverError:            true,
				PotentialUserAppError:           true,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
		{
			name: "no user app error",
			d: Detail{
				PotentialHWError:                false,
				PotentialDriverError:            true,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.IsOnlyUserAppError(); got != tt.want {
				t.Errorf("Detail.IsOnlyUserAppError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetail_IsOnlyDriverError(t *testing.T) {
	tests := []struct {
		name string
		d    Detail
		want bool
	}{
		{
			name: "only driver error",
			d: Detail{
				PotentialHWError:                false,
				PotentialDriverError:            true,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: true,
		},
		{
			name: "driver error with hardware error",
			d: Detail{
				PotentialHWError:                true,
				PotentialDriverError:            true,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
		{
			name: "no driver error",
			d: Detail{
				PotentialHWError:                true,
				PotentialDriverError:            false,
				PotentialUserAppError:           false,
				PotentialSystemMemoryCorruption: false,
				PotentialBusError:               false,
				PotentialThermalIssue:           false,
				PotentialFBCorruption:           false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.IsOnlyDriverError(); got != tt.want {
				t.Errorf("Detail.IsOnlyDriverError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetailsValidation(t *testing.T) {
	for _, d := range details {
		// test critical errors must have repair actions
		if d.CriticalErrorMarkedByGPUd && len(d.SuggestedActionsByGPUd.RepairActions) == 0 {
			t.Errorf("xid %d is marked as critical in GPUd, but has no repair actions", d.Xid)
		}

		// if nvidia says only possible reason is hw, then we do hard inspections directly
		if d.Xid != 48 && d.IsOnlyHWError() && (d.SuggestedActionsByGPUd == nil || len(d.SuggestedActionsByGPUd.RepairActions) == 0) {
			t.Fatalf("xid %d is only hardware error, but has no suggested actions", d.Xid)
		}
		if d.Xid != 48 && d.IsOnlyHWError() && d.SuggestedActionsByGPUd != nil {
			if d.SuggestedActionsByGPUd.RepairActions[0] != common.RepairActionTypeHardwareInspection {
				t.Errorf("xid %d is only hardware error, but has %s action", d.Xid, d.SuggestedActionsByGPUd.RepairActions[0])
			}
		}

		// if nvidia says this can be only because of user error, then we ignore, don’t mark it as critical
		if d.IsOnlyUserAppError() && d.CriticalErrorMarkedByGPUd {
			t.Errorf("xid %d is only user app error, but is marked as critical", d.Xid)
		}

		// if nvidia says this can be only because of driver error, then we only reboot
		if d.Xid != 38 && d.Xid != 44 && d.IsOnlyDriverError() && (d.SuggestedActionsByGPUd == nil || len(d.SuggestedActionsByGPUd.RepairActions) != 1) {
			t.Fatalf("xid %d is only driver error expecting 1 action, but %+v", d.Xid, d.SuggestedActionsByGPUd)
		}
		if d.Xid != 38 && d.Xid != 44 && d.IsOnlyDriverError() && d.SuggestedActionsByGPUd != nil {
			if d.SuggestedActionsByGPUd.RepairActions[0] != common.RepairActionTypeRebootSystem {
				t.Fatalf("xid %d is only driver error, but has %s action", d.Xid, d.SuggestedActionsByGPUd.RepairActions[0])
			}
		}

		if d.SuggestedActionsByGPUd != nil && len(d.SuggestedActionsByGPUd.Descriptions) > 0 &&
			len(d.SuggestedActionsByGPUd.Descriptions) != len(d.SuggestedActionsByGPUd.RepairActions) {
			t.Errorf("xid %d has %d descriptions and %d repair actions",
				d.Xid,
				len(d.SuggestedActionsByGPUd.Descriptions),
				len(d.SuggestedActionsByGPUd.RepairActions))
		}
	}
}
