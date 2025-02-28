package latency

import (
	"bytes"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLatencies_RenderTable(t *testing.T) {
	tests := []struct {
		name      string
		latencies Latencies
		want      []string // Strings that should be present in the output
	}{
		{
			name: "multiple latencies",
			latencies: Latencies{
				{
					Provider:            "provider1",
					RegionName:          "Region 1",
					RegionCode:          "reg1",
					Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
					LatencyMilliseconds: 100,
				},
				{
					Provider:            "provider2",
					RegionName:          "Region 2",
					RegionCode:          "reg2",
					Latency:             metav1.Duration{Duration: 200 * time.Millisecond},
					LatencyMilliseconds: 200,
				},
			},
			want: []string{
				"provider1", "Region 1", "reg1", "100ms",
				"provider2", "Region 2", "reg2", "200ms",
				"PROVIDER", "REGION NAME", "REGION CODE", "LATENCY", // Headers are in uppercase
			},
		},
		{
			name:      "empty latencies",
			latencies: Latencies{},
			want:      []string{"PROVIDER", "REGION NAME", "REGION CODE", "LATENCY"}, // Headers are in uppercase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.latencies.RenderTable(&buf)
			output := buf.String()

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("RenderTable() output doesn't contain expected string: %q", want)
				}
			}
		})
	}
}

func TestLatencies_Closest(t *testing.T) {
	tests := []struct {
		name      string
		latencies Latencies
		want      Latency
	}{
		{
			name: "multiple latencies",
			latencies: Latencies{
				{
					Provider:            "provider1",
					RegionName:          "Region 1",
					RegionCode:          "reg1",
					Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
					LatencyMilliseconds: 100,
				},
				{
					Provider:            "provider2",
					RegionName:          "Region 2",
					RegionCode:          "reg2",
					Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
					LatencyMilliseconds: 50,
				},
				{
					Provider:            "provider3",
					RegionName:          "Region 3",
					RegionCode:          "reg3",
					Latency:             metav1.Duration{Duration: 150 * time.Millisecond},
					LatencyMilliseconds: 150,
				},
			},
			want: Latency{
				Provider:            "provider2",
				RegionName:          "Region 2",
				RegionCode:          "reg2",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
		},
		{
			name: "single latency",
			latencies: Latencies{
				{
					Provider:            "provider1",
					RegionName:          "Region 1",
					RegionCode:          "reg1",
					Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
					LatencyMilliseconds: 100,
				},
			},
			want: Latency{
				Provider:            "provider1",
				RegionName:          "Region 1",
				RegionCode:          "reg1",
				Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
				LatencyMilliseconds: 100,
			},
		},
		{
			name: "first item with zero latency",
			latencies: Latencies{
				{
					Provider:            "provider1",
					RegionName:          "Region 1",
					RegionCode:          "reg1",
					Latency:             metav1.Duration{Duration: 0},
					LatencyMilliseconds: 0,
				},
				{
					Provider:            "provider2",
					RegionName:          "Region 2",
					RegionCode:          "reg2",
					Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
					LatencyMilliseconds: 50,
				},
			},
			want: Latency{
				Provider:            "provider2",
				RegionName:          "Region 2",
				RegionCode:          "reg2",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
		},
		{
			name: "all items with zero latency",
			latencies: Latencies{
				{
					Provider:            "provider1",
					RegionName:          "Region 1",
					RegionCode:          "reg1",
					Latency:             metav1.Duration{Duration: 0},
					LatencyMilliseconds: 0,
				},
				{
					Provider:            "provider2",
					RegionName:          "Region 2",
					RegionCode:          "reg2",
					Latency:             metav1.Duration{Duration: 0},
					LatencyMilliseconds: 0,
				},
			},
			want: Latency{
				Provider:            "provider2",
				RegionName:          "Region 2",
				RegionCode:          "reg2",
				Latency:             metav1.Duration{Duration: 0},
				LatencyMilliseconds: 0,
			},
		},
		{
			name:      "empty latencies",
			latencies: Latencies{},
			want:      Latency{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.latencies.Closest()

			// Compare individual fields since we can't compare structs directly due to Duration type
			if got.Provider != tt.want.Provider ||
				got.RegionName != tt.want.RegionName ||
				got.RegionCode != tt.want.RegionCode ||
				got.Latency.Duration != tt.want.Latency.Duration ||
				got.LatencyMilliseconds != tt.want.LatencyMilliseconds {
				t.Errorf("Closest() = %v, want %v", got, tt.want)
			}
		})
	}
}
