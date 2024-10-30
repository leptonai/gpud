package xid

import (
	"encoding/json"
	"reflect"
	"testing"

	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
)

func TestOutputEvaluate(t *testing.T) {
	tests := []struct {
		name        string
		input       *Output
		wantReason  Reason
		wantHealthy bool
		wantErr     bool
	}{
		{
			name:  "no errors",
			input: &Output{},
			wantReason: Reason{
				Messages: []string{"no xid error found"},
			},
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "nvml xid error",
			input: &Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Xid:                          79,
					XidCriticalErrorMarkedByNVML: true,
					XidCriticalErrorMarkedByGPUd: true,
					Detail: &nvidia_query_xid.Detail{
						Description: "GPU has fallen off the bus",
					},
				},
			},
			wantReason: Reason{
				Messages: []string{`xid event found from nvml:

detail:
  bus_error: false
  critical_error_marked_by_gpud: false
  description: GPU has fallen off the bus
  documentation_version: ""
  driver_error: false
  fb_corruption: false
  hw_error: false
  name: ""
  system_memory_corruption: false
  thermal_issue: false
  user_app_error: false
  xid: 0
device_uuid: ""
event_type: 0
sample_duration: 0s
time: null
xid: 79
xid_critical_error_marked_by_gpud: true
xid_critical_error_marked_by_nvml: true
`},
				Errors: map[uint64]XidError{
					79: {
						DataSource: "nvml",
						RawEvent: &nvidia_query_nvml.XidEvent{
							Xid:                          79,
							XidCriticalErrorMarkedByNVML: true,
							XidCriticalErrorMarkedByGPUd: true,
							Detail: &nvidia_query_xid.Detail{
								Description: "GPU has fallen off the bus",
							},
						},
						Xid:                          79,
						XidCriticalErrorMarkedByNVML: true,
						XidCriticalErrorMarkedByGPUd: true,
					},
				},
			},
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "dmesg xid error",
			input: &Output{
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						DetailFound: true,
						Detail: &nvidia_query_xid.Detail{
							XID:                       79,
							CriticalErrorMarkedByGPUd: true,
						},
					},
				},
			},
			wantReason: Reason{
				Messages: []string{
					"no xid error found",
					`xid error event found from dmesg:

- detail:
    bus_error: false
    critical_error_marked_by_gpud: true
    description: ""
    documentation_version: ""
    driver_error: false
    fb_corruption: false
    hw_error: false
    name: ""
    system_memory_corruption: false
    thermal_issue: false
    user_app_error: false
    xid: 79
  detail_found: true
  log_item:
    line: ""
    time: null
`},
				Errors: map[uint64]XidError{
					79: {
						DataSource:                   "dmesg",
						Xid:                          79,
						XidCriticalErrorMarkedByGPUd: true,
					},
				},
			},
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "both nvml and dmesg errors",
			input: &Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Xid:                          79,
					XidCriticalErrorMarkedByNVML: true,
					XidCriticalErrorMarkedByGPUd: true,
				},
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						DetailFound: true,
						Detail: &nvidia_query_xid.Detail{
							XID:                       79,
							CriticalErrorMarkedByGPUd: true,
						},
					},
					{
						DetailFound: true,
						Detail: &nvidia_query_xid.Detail{
							XID:                       80,
							CriticalErrorMarkedByGPUd: true,
						},
					},
				},
			},
			wantReason: Reason{
				Messages: []string{
					`xid event found from nvml:

device_uuid: ""
event_type: 0
sample_duration: 0s
time: null
xid: 79
xid_critical_error_marked_by_gpud: true
xid_critical_error_marked_by_nvml: true
`,
					`xid error event found from dmesg:

- detail:
    bus_error: false
    critical_error_marked_by_gpud: true
    description: ""
    documentation_version: ""
    driver_error: false
    fb_corruption: false
    hw_error: false
    name: ""
    system_memory_corruption: false
    thermal_issue: false
    user_app_error: false
    xid: 79
  detail_found: true
  log_item:
    line: ""
    time: null
- detail:
    bus_error: false
    critical_error_marked_by_gpud: true
    description: ""
    documentation_version: ""
    driver_error: false
    fb_corruption: false
    hw_error: false
    name: ""
    system_memory_corruption: false
    thermal_issue: false
    user_app_error: false
    xid: 80
  detail_found: true
  log_item:
    line: ""
    time: null
`},
				Errors: map[uint64]XidError{
					79: {
						DataSource: "nvml",
						RawEvent: &nvidia_query_nvml.XidEvent{
							Xid:                          79,
							XidCriticalErrorMarkedByNVML: true,
							XidCriticalErrorMarkedByGPUd: true,
						},
						Xid:                          79,
						XidCriticalErrorMarkedByNVML: true,
						XidCriticalErrorMarkedByGPUd: true,
					},
					80: {
						DataSource:                   "dmesg",
						Xid:                          80,
						XidCriticalErrorMarkedByGPUd: true,
					},
				},
			},
			wantHealthy: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReason, gotHealthy, err := tt.input.Evaluate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Output.Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Compare the actual structs instead of JSON representations
			if !reflect.DeepEqual(gotReason, tt.wantReason) {
				// For better error messages, still use JSON for output
				gotJSON, _ := json.MarshalIndent(gotReason, "", "  ")
				wantJSON, _ := json.MarshalIndent(tt.wantReason, "", "  ")
				t.Errorf("Output.Evaluate() reason = \n%s\n\nwant\n%s", string(gotJSON), string(wantJSON))
			}

			if gotHealthy != tt.wantHealthy {
				t.Errorf("Output.Evaluate() healthy = %v, want %v", gotHealthy, tt.wantHealthy)
			}
		})
	}
}
