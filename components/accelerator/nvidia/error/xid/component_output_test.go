package xid

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	query_log "github.com/leptonai/gpud/components/query/log"

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
			name:  "no errors",
			input: &Output{},
			wantReason: Reason{
				Messages: []string{"no xid error found"},
			},
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
					fmt.Sprintf("xid 79 detected by NVML (%s)", humanize.Time(ts.UTC())),
				},
				Errors: map[uint64]XidError{
					79: {
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
				Errors: map[uint64]XidError{
					79: {
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
					fmt.Sprintf("xid 79 detected by NVML (%s)", humanize.Time(ts.UTC())),
					fmt.Sprintf("xid 80 detected by dmesg (%s)", humanize.Time(ts.UTC())),
				},
				Errors: map[uint64]XidError{
					79: {
						Time:                      metav1.Time{Time: ts},
						DataSource:                "nvml",
						Xid:                       79,
						CriticalErrorMarkedByGPUd: true,
					},
					80: {
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

func TestOutput_GetReason_ErrorPrioritization(t *testing.T) {
	testTime := time.Now()
	olderTime := metav1.Time{Time: testTime.Add(-1 * time.Hour)}
	newerTime := metav1.Time{Time: testTime.Add(1 * time.Hour)}

	tests := []struct {
		name     string
		input    Output
		wantXid  uint64
		wantSrc  string
		wantTime time.Time
	}{
		{
			name: "prefer nvml over older dmesg",
			input: Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Xid:  123,
					Time: metav1.Time{Time: testTime},
				},
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						LogItem: query_log.Item{Time: olderTime},
						Detail:  &nvidia_query_xid.Detail{Xid: 123},
					},
				},
			},
			wantXid:  123,
			wantSrc:  "nvml",
			wantTime: testTime,
		},
		{
			name: "prefer nvml over newer dmesg",
			input: Output{
				NVMLXidEvent: &nvidia_query_nvml.XidEvent{
					Xid:  123,
					Time: metav1.Time{Time: testTime},
				},
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						LogItem: query_log.Item{Time: newerTime},
						Detail:  &nvidia_query_xid.Detail{Xid: 123},
					},
				},
			},
			wantXid:  123,
			wantSrc:  "nvml",
			wantTime: testTime,
		},
		{
			name: "prefer newer dmesg when no nvml",
			input: Output{
				DmesgErrors: []nvidia_query_xid.DmesgError{
					{
						LogItem: query_log.Item{Time: olderTime},
						Detail:  &nvidia_query_xid.Detail{Xid: 123},
					},
					{
						LogItem: query_log.Item{Time: newerTime},
						Detail:  &nvidia_query_xid.Detail{Xid: 123},
					},
				},
			},
			wantXid:  123,
			wantSrc:  "dmesg",
			wantTime: newerTime.Time,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := tt.input.GetReason()

			if len(reason.Errors) != 1 {
				t.Errorf("expected 1 error, got %d", len(reason.Errors))
				return
			}

			err := reason.Errors[tt.wantXid]
			if err.Xid != tt.wantXid {
				t.Errorf("wrong XID, want %d, got %d", tt.wantXid, err.Xid)
			}
			if err.DataSource != tt.wantSrc {
				t.Errorf("wrong source, want %s, got %s", tt.wantSrc, err.DataSource)
			}
			if !err.Time.Time.Equal(tt.wantTime) {
				t.Errorf("wrong time, want %v, got %v", tt.wantTime, err.Time)
			}
		})
	}
}
