package latency_test

import (
	"testing"

	"github.com/leptonai/gpud/components/network/latency"
	pkg_latency "github.com/leptonai/gpud/pkg/netutil/latency"
)

func TestStatesHealthyEvaluation(t *testing.T) {
	tests := []struct {
		name                  string
		latencies             pkg_latency.Latencies
		globalThreshold       int64
		expectedHealthyStatus bool
	}{
		{
			name: "All latencies below threshold",
			latencies: pkg_latency.Latencies{
				{LatencyMilliseconds: 50, RegionName: "region1"},
				{LatencyMilliseconds: 60, RegionName: "region2"},
			},
			globalThreshold:       100,
			expectedHealthyStatus: true,
		},
		{
			name: "Some latencies above threshold",
			latencies: pkg_latency.Latencies{
				{LatencyMilliseconds: 150, RegionName: "region1"},
				{LatencyMilliseconds: 60, RegionName: "region2"},
			},
			globalThreshold:       100,
			expectedHealthyStatus: true,
		},
		{
			name: "All latencies above threshold",
			latencies: pkg_latency.Latencies{
				{LatencyMilliseconds: 150, RegionName: "region1"},
				{LatencyMilliseconds: 160, RegionName: "region2"},
			},
			globalThreshold:       100,
			expectedHealthyStatus: false,
		},
		{
			name: "No threshold set",
			latencies: pkg_latency.Latencies{
				{LatencyMilliseconds: 150, RegionName: "region1"},
				{LatencyMilliseconds: 160, RegionName: "region2"},
			},
			globalThreshold:       0,
			expectedHealthyStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &latency.Output{
				EgressLatencies: tt.latencies,
			}
			cfg := latency.Config{
				GlobalMillisecondThreshold: tt.globalThreshold,
			}

			states, err := output.States(cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(states) != 1 {
				t.Fatalf("expected 1 state, got %d", len(states))
			}

			if states[0].Healthy != tt.expectedHealthyStatus {
				t.Errorf("expected healthy status to be %v, got %v", tt.expectedHealthyStatus, states[0].Healthy)
			}
		})
	}
}
