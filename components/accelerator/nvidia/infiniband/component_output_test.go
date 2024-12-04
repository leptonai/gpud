package infiniband

import (
	"testing"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
)

func TestValidateIBPorts(t *testing.T) {
	tests := []struct {
		name           string
		cards          []infiniband.IBStatCard
		gpuCount       int
		gpuProductName string
		expectedStates ExpectedPortStates
		wantMsg        string
		wantOK         bool
	}{
		{
			name: "all ports active and matching rate",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
			},
			gpuCount:       4,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 4,
				Rate:      200,
			},
			wantMsg: "",
			wantOK:  true,
		},
		{
			name: "some ports down",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "LinkDown", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "LinkDown", Rate: 200}},
			},
			gpuCount:       4,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 4,
				Rate:      200,
			},
			wantMsg: "only 2 out of 4 ibstat cards are link up and active (expected rate: 200 Gb/sec)",
			wantOK:  false,
		},
		{
			name: "wrong rate",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100}},
			},
			gpuCount:       2,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 2,
				Rate:      200,
			},
			wantMsg: "only 0 out of 2 ibstat cards are link up and active (expected rate: 200 Gb/sec)",
			wantOK:  false,
		},
		{
			name: "zero port count defaults to gpu count",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
			},
			gpuCount:       2,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 0,
				Rate:      200,
			},
			wantMsg: "",
			wantOK:  true,
		},
		{
			name: "zero rate uses default for GPU model",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
			},
			gpuCount:       2,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 2,
				Rate:      1,
			},
			wantMsg: "",
			wantOK:  true,
		},
		{
			name: "port count greater than GPU count uses GPU count",
			cards: []infiniband.IBStatCard{
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
			},
			gpuCount:       2,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 4,
				Rate:      200,
			},
			wantMsg: "",
			wantOK:  true,
		},
		{
			name:           "empty cards",
			cards:          []infiniband.IBStatCard{},
			gpuCount:       2,
			gpuProductName: "H100",
			expectedStates: ExpectedPortStates{
				PortCount: 2,
				Rate:      200,
			},
			wantMsg: "only 0 out of 2 ibstat cards are link up and active (expected rate: 200 Gb/sec)",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMsg, gotOK := validateIBPorts(tt.cards, tt.gpuCount, tt.gpuProductName, tt.expectedStates)

			if gotMsg != tt.wantMsg {
				t.Errorf("validateIBPorts() message = %q, want %q", gotMsg, tt.wantMsg)
			}
			if gotOK != tt.wantOK {
				t.Errorf("validateIBPorts() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}
