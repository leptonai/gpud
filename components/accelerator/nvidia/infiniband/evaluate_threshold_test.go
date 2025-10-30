package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// TestCheckWithEmptyIbportsToEvaluate tests Check when ibportsToEvaluate ends up empty after processing ClassDevices
func TestCheckWithEmptyIbportsToEvaluate(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create mock devices without ports to test empty ibportsToEvaluate path
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000123",
			FirmwareVersion: "28.40.1000",
			HCAType:         "MT4125",
			Ports:           []infinibandclass.Port{}, // Empty ports
		},
	}

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
}

// TestEvaluateHealthStateWithThresholdsComprehensive tests more edge cases for evaluateHealthStateWithThresholds
func TestEvaluateHealthStateWithThresholdsComprehensive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		thresholds        types.ExpectedPortStates
		ibports           []types.IBPort
		expectedHealth    apiv1.HealthStateType
		expectedReason    string
		shouldHaveActions bool
		shouldHaveError   bool
	}{
		{
			name:           "zero thresholds should return healthy with no threshold reason",
			thresholds:     types.ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 0},
			ibports:        []types.IBPort{},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoThreshold,
		},
		{
			name:           "empty ibports should return healthy with no data reason",
			thresholds:     types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			ibports:        []types.IBPort{},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbPortData,
		},
		{
			name:       "healthy ports meeting thresholds",
			thresholds: types.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 200},
			ibports: []types.IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbPortIssue,
		},
		{
			name:       "insufficient ports should NOT suggest hardware inspection",
			thresholds: types.ExpectedPortStates{AtLeastPorts: 4, AtLeastRate: 200},
			ibports: []types.IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
			},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReason:    "only 2 port(s) are active and >=200 Gb/s, expect >=4 port(s)",
			shouldHaveActions: false, // No hardware inspection for port count mismatch
		},
		{
			name:       "insufficient rate should NOT suggest hardware inspection",
			thresholds: types.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400},
			ibports: []types.IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
			},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReason:    "only 0 port(s) are active and >=400 Gb/s, expect >=2 port(s)",
			shouldHaveActions: false, // No hardware inspection for rate mismatch
		},
		{
			name:       "disabled ports should NOT suggest hardware inspection",
			thresholds: types.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 200},
			ibports: []types.IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
				{Device: "mlx5_1", State: "Down", PhysicalState: "Disabled", RateGBSec: 0, LinkLayer: "Infiniband"},
			},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReason:    "only 1 port(s) are active and >=200 Gb/s, expect >=2 port(s); 1 device(s) physical state Disabled (mlx5_1)",
			shouldHaveActions: false, // No hardware inspection for disabled ports
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &checkResult{
				ts: time.Now().UTC(),
			}

			evaluateHealthStateWithThresholds(tt.thresholds, tt.ibports, cr)

			assert.Equal(t, tt.expectedHealth, cr.health)
			assert.Equal(t, tt.expectedReason, cr.reason)

			if tt.shouldHaveActions {
				assert.NotNil(t, cr.suggestedActions)
			} else {
				assert.Nil(t, cr.suggestedActions)
			}

			if tt.shouldHaveError {
				assert.NotNil(t, cr.err)
			} else {
				assert.Nil(t, cr.err)
			}
		})
	}
}
