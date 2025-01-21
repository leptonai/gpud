package infiniband

import (
	"testing"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
)

func TestOutputEvaluateEmptyConfig(t *testing.T) {
	o := &Output{
		GPUProductName: "NVIDIA A100",
	}
	cfg := ExpectedPortStates{}
	reason, healthy, err := o.Evaluate(cfg)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !healthy {
		t.Errorf("Evaluate() healthy = false, reason = %s", reason)
	}
	if reason != msgNoAtLeastPortsOrRateSet {
		t.Errorf("Evaluate() reason = %s, want %s", reason, msgNoAtLeastPortsOrRateSet)
	}
}

func TestOutputEvaluateH100(t *testing.T) {
	o := &Output{
		GPUProductName: "NVIDIA H100",
		IbstatExists:   true,
		Ibstat: infiniband.IbstatOutput{
			Parsed: []infiniband.IBStatCard{
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
			},
		},
	}
	cfg := ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	}
	reason, healthy, err := o.Evaluate(cfg)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !healthy {
		t.Errorf("Evaluate() healthy = false, reason = %s", reason)
	}
	if reason != msgNoIbstatIssueFound {
		t.Errorf("Evaluate() reason = %s, want %s", reason, msgNoIbstatIssueFound)
	}
	t.Logf("reason: %s", reason)
}

func TestOutputEvaluateNoIbstatExists(t *testing.T) {
	o := &Output{
		GPUProductName: "NVIDIA H100",
		IbstatExists:   false,
	}
	cfg := ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	}
	reason, healthy, err := o.Evaluate(cfg)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if healthy {
		t.Errorf("Evaluate() healthy = true, reason = %s", reason)
	}
	if reason != msgNoIbstatExists {
		t.Errorf("Evaluate() reason = %s, want %s", reason, msgNoIbstatExists)
	}
	t.Logf("reason: %s", reason)
}

func TestOutputEvaluateNoIbstatDataFound(t *testing.T) {
	o := &Output{
		GPUProductName: "NVIDIA H100",
		IbstatExists:   true,
		Ibstat: infiniband.IbstatOutput{
			Parsed: []infiniband.IBStatCard{},
		},
	}
	cfg := ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	}
	reason, healthy, err := o.Evaluate(cfg)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if healthy {
		t.Errorf("Evaluate() healthy = true, reason = %s", reason)
	}
	if reason != msgNoIbstatDataFound {
		t.Errorf("Evaluate() reason = %s, want %s", reason, msgNoIbstatDataFound)
	}
	t.Logf("reason: %s", reason)
}

func TestOutputEvaluateH100MissingPort(t *testing.T) {
	o := &Output{
		GPUProductName: "NVIDIA H100",
		IbstatExists:   true,
		Ibstat: infiniband.IbstatOutput{
			Parsed: []infiniband.IBStatCard{
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Port1: infiniband.IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
			},
		},
	}
	cfg := ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	}
	reason, healthy, err := o.Evaluate(cfg)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if healthy {
		t.Errorf("Evaluate() healthy = true, reason = %s", reason)
	}
	t.Logf("reason: %s", reason)
}
