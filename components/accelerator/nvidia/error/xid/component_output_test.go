package xid

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	query_log "github.com/leptonai/gpud/poller/log"

	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOutputGetReason(t *testing.T) {
	ts := time.Now().Add(-999 * time.Hour)

	tests := []struct {
		name       string
		input      *Output
		wantReason Reason
	}{
		{
			name:       "no errors",
			input:      &Output{},
			wantReason: Reason{},
		},
		{
			name: "nvml xid error",
			input: &Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Time: metav1.Time{Time: ts},
					Xid:  79,
					Detail: &nvidia_query_xid.Detail{
						Description:               "GPU has fallen off the bus",
						CriticalErrorMarkedByGPUd: true,
					},
				},
			},
			wantReason: Reason{
				Messages: []string{
					fmt.Sprintf("xid 79 detected by nvml (%s)", humanize.Time(ts.UTC())),
				},
				Errors: []XidError{
					{
						Time:                      metav1.Time{Time: ts},
						DataSource:                "nvml",
						Xid:                       79,
						CriticalErrorMarkedByGPUd: true,
					},
				},
			},
		},
		{
			name: "dmesg xid error",
			input: &Output{
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						LogItem: query_log.Item{
							Time: metav1.Time{Time: ts},
						},
						Detail: &nvidia_query_xid.Detail{
							Xid:                       79,
							CriticalErrorMarkedByGPUd: true,
						},
					},
				},
			},
			wantReason: Reason{
				Messages: []string{
					fmt.Sprintf("xid 79 detected by dmesg (%s)", humanize.Time(ts.UTC())),
				},
				Errors: []XidError{
					{
						Time:                      metav1.Time{Time: ts},
						DataSource:                "dmesg",
						Xid:                       79,
						CriticalErrorMarkedByGPUd: true,
					},
				},
			},
		},
		{
			name: "both nvml and dmesg errors",
			input: &Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Time: metav1.Time{Time: ts},
					Xid:  79,
					Detail: &nvidia_query_xid.Detail{
						CriticalErrorMarkedByGPUd: true,
					},
				},
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						LogItem: query_log.Item{
							Time: metav1.Time{Time: ts},
						},
						Detail: &nvidia_query_xid.Detail{
							Xid:                       79,
							CriticalErrorMarkedByGPUd: true,
						},
					},
					{
						LogItem: query_log.Item{
							Time: metav1.Time{Time: ts},
						},
						Detail: &nvidia_query_xid.Detail{
							Xid:                       80,
							CriticalErrorMarkedByGPUd: true,
						},
					},
				},
			},
			wantReason: Reason{
				Messages: []string{
					fmt.Sprintf("xid 79 detected by nvml (%s)", humanize.Time(ts.UTC())),
					fmt.Sprintf("xid 79 detected by dmesg (%s)", humanize.Time(ts.UTC())),
					fmt.Sprintf("xid 80 detected by dmesg (%s)", humanize.Time(ts.UTC())),
				},
				Errors: []XidError{
					{
						Time:                      metav1.Time{Time: ts},
						DataSource:                "nvml",
						Xid:                       79,
						CriticalErrorMarkedByGPUd: true,
					},
					{
						Time:                      metav1.Time{Time: ts},
						DataSource:                "dmesg",
						Xid:                       79,
						CriticalErrorMarkedByGPUd: true,
					},
					{
						Time:                      metav1.Time{Time: ts},
						DataSource:                "dmesg",
						Xid:                       80,
						CriticalErrorMarkedByGPUd: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReason := tt.input.GetReason()

			// Compare the actual structs instead of JSON representations
			if !reflect.DeepEqual(gotReason, tt.wantReason) {
				// For better error messages, still use JSON for output
				gotJSON, _ := json.MarshalIndent(gotReason, "", "  ")
				wantJSON, _ := json.MarshalIndent(tt.wantReason, "", "  ")
				t.Errorf("Output.GetReason() = \n%s\n\nwant\n%s", string(gotJSON), string(wantJSON))
			}
		})
	}
}
