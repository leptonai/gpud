package nvlink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "existing reason", cr.reason)
}

func TestEvaluateThresholds_NoData(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	cr := &checkResult{
		ExpectedLinkStates: &thresholds,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, reasonNoNVLinkData, cr.reason)
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

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.NotEmpty(t, cr.reason)
	assert.NotEqual(t, reasonNoNVLinkData, cr.reason)
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

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.NotEmpty(t, cr.reason)
	assert.NotEqual(t, reasonNoNVLinkData, cr.reason)
	assert.Contains(t, cr.reason, "inactive nvlinks=GPU-0")
	require.NotNil(t, cr.suggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
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

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "unsupported nvlinks=GPU-1")
	assert.Nil(t, cr.suggestedActions)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "nvlink threshold violated")
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "inactive nvlinks=GPU-1")
	assert.Contains(t, cr.reason, "unsupported nvlinks=GPU-0")
	assert.Contains(t, cr.reason, "require >=2")
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
	assert.Equal(t, "existing reason", cr.reason)

	// Verify that the threshold is considered zero/unset
	assert.True(t, thresholds.IsZero())
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "satisfied")
}

func TestEvaluateThresholds_ImplicitFailureWhenSystemExpectedNVLink(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: false,
			},
			{
				UUID:      "GPU-1",
				Supported: false,
			},
		},
		UnsupportedNVLinkUUIDs: []string{"GPU-0", "GPU-1"},
		SystemExpectedNVLink:   true,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "no GPUs report active nvlink links")
	assert.Contains(t, cr.reason, "unsupported nvlinks=GPU-0,GPU-1")
	assert.Nil(t, cr.suggestedActions)
}

func TestEvaluateThresholds_ImplicitFailureWhenPeerNVLinkP2PReportsNoOKPairs(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:             []string{"GPU-0", "GPU-1"},
		PeerNVLinkProbePairCount:      1,
		PeerNVLinkExpectedPairCount:   1,
		PeerNVLinkOKPairCount:         0,
		PeerNVLinkObservedStatusCodes: []string{p2pStatusNotSupported},
		SystemExpectedNVLink:          true,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "no GPU pairs report NVLink P2P connectivity")
	assert.Contains(t, cr.reason, "peer nvlink p2p statuses=NS")
	assert.Nil(t, cr.suggestedActions)
}

func TestEvaluateThresholds_ImplicitPeerFailureSkippedWhenProbeCoverageIsPartial(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-2",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:             []string{"GPU-0", "GPU-1", "GPU-2"},
		PeerNVLinkProbePairCount:      1,
		PeerNVLinkExpectedPairCount:   3,
		PeerNVLinkOKPairCount:         0,
		PeerNVLinkObservedStatusCodes: []string{p2pStatusNotSupported},
		SystemExpectedNVLink:          true,
		health:                        apiv1.HealthStateTypeHealthy,
		reason:                        "all 3 GPU(s) were checked, no nvlink issue found",
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 3 GPU(s) were checked, no nvlink issue found", cr.reason)
	assert.Nil(t, cr.suggestedActions)
}

func TestEvaluateThresholds_ConfiguredThresholdStillFailsOnPeerNVLinkP2PFailure(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 2}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:             []string{"GPU-0", "GPU-1"},
		PeerNVLinkProbePairCount:      1,
		PeerNVLinkExpectedPairCount:   1,
		PeerNVLinkOKPairCount:         0,
		PeerNVLinkObservedStatusCodes: []string{p2pStatusNotSupported},
		ExpectedLinkStates:            &thresholds,
		SystemExpectedNVLink:          true,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "no GPU pairs report NVLink P2P connectivity")
	assert.Nil(t, cr.suggestedActions)
}

func TestEvaluateThresholds_PeerNVLinkUnknownStatusSuggestsReboot(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
		},
		ActiveNVLinkUUIDs:             []string{"GPU-0", "GPU-1"},
		PeerNVLinkProbePairCount:      1,
		PeerNVLinkExpectedPairCount:   1,
		PeerNVLinkOKPairCount:         0,
		PeerNVLinkObservedStatusCodes: []string{p2pStatusUnknown},
		SystemExpectedNVLink:          true,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.NotNil(t, cr.suggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
}

func TestEvaluateThresholds_ImplicitFallbackDoesNotFailPartialDegradation(t *testing.T) {
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: false},
				},
			},
		},
		ActiveNVLinkUUIDs:    []string{"GPU-0"},
		InactiveNVLinkUUIDs:  []string{"GPU-1"},
		SystemExpectedNVLink: true,
		health:               apiv1.HealthStateTypeHealthy,
		reason:               "all 2 GPU(s) were checked, no nvlink issue found",
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 2 GPU(s) were checked, no nvlink issue found", cr.reason)
}

func TestEvaluateThresholds_PartialDegradationFailsWhenThresholdConfigured(t *testing.T) {
	thresholds := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 2}
	cr := &checkResult{
		NVLinks: []NVLink{
			{
				UUID:      "GPU-0",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			},
			{
				UUID:      "GPU-1",
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: false},
				},
			},
		},
		ActiveNVLinkUUIDs:    []string{"GPU-0"},
		InactiveNVLinkUUIDs:  []string{"GPU-1"},
		ExpectedLinkStates:   &thresholds,
		SystemExpectedNVLink: true,
	}

	evaluateHealthStateWithThresholds(cr)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "require >=2 GPUs with all links active; got 1")
	assert.Contains(t, cr.reason, "inactive nvlinks=GPU-1")
}
