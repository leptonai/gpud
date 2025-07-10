package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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
		thresholds        infiniband.ExpectedPortStates
		ibports           []infiniband.IBPort
		expectedHealth    apiv1.HealthStateType
		expectedReason    string
		shouldHaveActions bool
		shouldHaveError   bool
	}{
		{
			name:           "zero thresholds should return healthy with no threshold reason",
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 0},
			ibports:        []infiniband.IBPort{},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoThreshold,
		},
		{
			name:           "empty ibports should return healthy with no data reason",
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			ibports:        []infiniband.IBPort{},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbPortData,
		},
		{
			name:       "healthy ports meeting thresholds",
			thresholds: infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 200},
			ibports: []infiniband.IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", RateGBSec: 200, LinkLayer: "Infiniband"},
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbPortIssue,
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
