package nvlink

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestEvaluateThresholds_ZeroThreshold(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{UUID: "GPU-0"},
		},
		ExpectedLinkStates: &ExpectedLinkStates{},
		health:             apiv1.HealthStateTypeHealthy,
		reason:             "existing reason",
	}

	evaluateHealthStateWithThresholds(cr)

	if cr.health != apiv1.HealthStateTypeHealthy {
		t.Fatalf("expected health to remain healthy, got %q", cr.health)
	}
	if cr.reason != "existing reason" {
		t.Fatalf("expected reason to remain unchanged, got %q", cr.reason)
	}
}

func TestEvaluateThresholds_NoData(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		ExpectedLinkStates: &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	if cr.health != apiv1.HealthStateTypeHealthy {
		t.Fatalf("expected healthy when no data, got %q", cr.health)
	}
	if cr.reason != reasonNoNVLinkData {
		t.Fatalf("expected reason %q, got %q", reasonNoNVLinkData, cr.reason)
	}
}

func TestEvaluateThresholds_Satisfied(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:  []string{"GPU-0"},
		ExpectedLinkStates: &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	if cr.health != apiv1.HealthStateTypeHealthy {
		t.Fatalf("expected healthy, got %q", cr.health)
	}
	if cr.reason == "" || cr.reason == reasonNoNVLinkData {
		t.Fatalf("expected informative reason, got %q", cr.reason)
	}
}

func TestEvaluateThresholds_ViolationInactive(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: false},
				},
			},
		},
		InactiveNVLinkUUIDs: []string{"GPU-0"},
		ExpectedLinkStates:  &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	if cr.health != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy, got %q", cr.health)
	}
	if cr.reason == "" || cr.reason == reasonNoNVLinkData {
		t.Fatalf("expected violation reason, got %q", cr.reason)
	}
	if want := "inactive nvlinks=GPU-0"; !strings.Contains(cr.reason, want) {
		t.Fatalf("expected reason to contain %q, got %q", want, cr.reason)
	}
	if cr.suggestedActions == nil {
		t.Fatalf("expected suggested actions when nvlink is inactive")
	}
	if diff := cmp.Diff([]apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions); diff != "" {
		t.Fatalf("unexpected suggested actions (-want +got):\n%s", diff)
	}
}

func TestEvaluateThresholds_ViolationUnsupported(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-1",
				Supported: false,
			},
		},
		UnsupportedNVLinkUUIDs: []string{"GPU-1"},
		ExpectedLinkStates:     &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	if cr.health != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy, got %q", cr.health)
	}
	if want := "unsupported nvlinks=GPU-1"; !strings.Contains(cr.reason, want) {
		t.Fatalf("expected reason to contain %q, got %q", want, cr.reason)
	}
	if cr.suggestedActions != nil {
		t.Fatalf("did not expect suggested actions when nvlink is unsupported")
	}
}

func TestEvaluateThresholds_EmptyStates(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States:    []NVLinkState{}, // Empty states
			},
		},
		ExpectedLinkStates: &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	// Should be unhealthy - 0 active GPUs < 1 required
	if cr.health != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy for GPU with empty states, got %q", cr.health)
	}
	if !strings.Contains(cr.reason, "nvlink threshold violated") {
		t.Fatalf("expected threshold violation in reason, got %q", cr.reason)
	}
}

func TestEvaluateThresholds_MixedInactiveAndUnsupported(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 2}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: false,
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: false},
				},
			},
			{
				UUID:      "GPU-2",
				Supported: true,
				States:    []NVLinkState{}, // Empty
			},
		},
		InactiveNVLinkUUIDs:    []string{"GPU-1"},
		UnsupportedNVLinkUUIDs: []string{"GPU-0"},
		ExpectedLinkStates:     &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	// Should be unhealthy - 0 active GPUs < 2 required
	if cr.health != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy, got %q", cr.health)
	}

	// Verify both types appear in reason
	if !strings.Contains(cr.reason, "inactive nvlinks=GPU-1") {
		t.Errorf("missing inactive GPUs in reason: %q", cr.reason)
	}
	if !strings.Contains(cr.reason, "unsupported nvlinks=GPU-0") {
		t.Errorf("missing unsupported GPUs in reason: %q", cr.reason)
	}
	if !strings.Contains(cr.reason, "require >=2") {
		t.Errorf("missing threshold requirement in reason: %q", cr.reason)
	}
}

func TestEvaluateThresholds_NegativeThresholdTreatedAsUnset(t *testing.T) {
	// Test that negative thresholds are treated as unset and skip evaluation
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: -1}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States:    []NVLinkState{}, // Empty states - would fail if evaluated
			},
		},
		ExpectedLinkStates: &thresholds,
		reason:             "existing reason",
	}

	evaluateHealthStateWithThresholds(cr)

	// Should keep existing state since threshold is treated as unset
	if cr.reason != "existing reason" {
		t.Fatalf("expected reason to remain unchanged for negative threshold, got %q", cr.reason)
	}

	// Verify that the threshold is considered zero/unset
	if !thresholds.IsZero() {
		t.Fatalf("negative threshold should be treated as IsZero")
	}
}

func TestEvaluateThresholds_MultipleGPUsPartialActive(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 2}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
					{FeatureEnabled: false}, // One link disabled
				},
			},
			{
				UUID:      "GPU-2",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:   []string{"GPU-0", "GPU-2"},
		InactiveNVLinkUUIDs: []string{"GPU-1"},
		ExpectedLinkStates:  &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	// Should be healthy - 2 GPUs have all links active (GPU-0 and GPU-2)
	if cr.health != apiv1.HealthStateTypeHealthy {
		t.Fatalf("expected healthy with 2 fully active GPUs, got %q", cr.health)
	}
	if !strings.Contains(cr.reason, "satisfied") {
		t.Fatalf("expected satisfied in reason, got %q", cr.reason)
	}
}
