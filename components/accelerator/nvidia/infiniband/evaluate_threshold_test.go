package infiniband

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		output     *infiniband.IbstatOutput
		config     infiniband.ExpectedPortStates
		wantReason string
		wantHealth apiv1.HealthStateType
	}{
		{
			name:   "thresholds not set",
			output: &infiniband.IbstatOutput{},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantReason: reasonNoThreshold,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name:   "only ports threshold set",
			output: &infiniband.IbstatOutput{Parsed: infiniband.IBStatCards{}},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  0,
			},
			wantReason: reasonNoThreshold,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name:   "only rate threshold set",
			output: &infiniband.IbstatOutput{Parsed: infiniband.IBStatCards{}},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  200,
			},
			wantReason: reasonNoThreshold,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "healthy state with matching ports and rate",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbPortIssue,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state - not enough ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 1 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - rate too low",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - disabled ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 port(s) are active and >=200 Gb/s, expect >=2 port(s); 2 device(s) physical state Disabled (mlx5_0, mlx5_1)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "empty ibstat cards",
			output: &infiniband.IbstatOutput{
				Raw:    "",
				Parsed: infiniband.IBStatCards{},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbPortData,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "inactive ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbPortIssue,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "mixed port states",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_2",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 3,
				AtLeastRate:  200,
			},
			wantReason: "only 2 port(s) are active and >=200 Gb/s, expect >=3 port(s); 1 device(s) physical state Disabled (mlx5_1)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "mixed rate values",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          400,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_2",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  300,
			},
			wantReason: "only 1 port(s) are active and >=300 Gb/s, expect >=2 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "zero rate value",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          0,
							LinkLayer:     "Infiniband",
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			wantReason: "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip nil output test to avoid panic
			if tt.output == nil {
				t.Skip("Skipping test with nil output")
				return
			}

			// Create a checkResult and populate it like the component does
			cr := &checkResult{
				IbstatOutput: tt.output,
			}

			var ibportsToEvaluate []infiniband.IBPort
			if tt.output != nil {
				ibportsToEvaluate = tt.output.Parsed.IBPorts()
			}

			// Call evaluateHealthStateWithThresholds which modifies cr in place
			evaluateHealthStateWithThresholds(tt.config, ibportsToEvaluate, cr)

			// Extract the results from the checkResult
			health := cr.health
			suggestedActions := cr.suggestedActions
			reason := cr.reason

			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, suggestedActions)
				if suggestedActions != nil {
					assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, suggestedActions.RepairActions)
				}
			}
		})
	}
}

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
		ctx:            cctx,
		cancel:         ccancel,
		getTimeNowFunc: mockTimeNow(),
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:    mockBucket,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return error to avoid early return and test the evaluateHealthStateWithThresholds path
			return nil, infiniband.ErrNoIbstatCommand
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

func TestEvaluateWithTestData(t *testing.T) {
	testDataPath := "../../../../pkg/nvidia-query/infiniband/testdata/ibstat.47.0.h100.all.active.1"
	content, err := os.ReadFile(testDataPath)
	require.NoError(t, err, "Failed to read test data file")

	cards, err := infiniband.ParseIBStat(string(content))
	require.NoError(t, err, "Failed to parse ibstat output")

	output := &infiniband.IbstatOutput{
		Raw:    string(content),
		Parsed: cards,
	}

	tests := []struct {
		name       string
		config     infiniband.ExpectedPortStates
		wantReason string
		wantHealth apiv1.HealthStateType
	}{
		{
			name: "unhealthy state - all ports are Ethernet, not InfiniBand",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 8,   // Number of 400Gb/s ports in the test data
				AtLeastRate:  400, // Expected rate for H100 cards
			},
			wantReason: "only 0 port(s) are active and >=400 Gb/s, expect >=8 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - all ports are Ethernet, not InfiniBand (mixed rate)",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports in test data
				AtLeastRate:  100, // Minimum rate that includes all ports
			},
			wantReason: "only 0 port(s) are active and >=100 Gb/s, expect >=12 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - all ports are Ethernet, not InfiniBand (high rate)",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports
				AtLeastRate:  400, // Only 8 ports have this rate
			},
			wantReason: "only 0 port(s) are active and >=400 Gb/s, expect >=12 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a checkResult and populate it like the component does
			cr := &checkResult{
				IbstatOutput: output,
			}

			var ibportsToEvaluate []infiniband.IBPort
			if output != nil {
				ibportsToEvaluate = output.Parsed.IBPorts()
			}

			// Call evaluateHealthStateWithThresholds which modifies cr in place
			evaluateHealthStateWithThresholds(tt.config, ibportsToEvaluate, cr)

			// Extract the results from the checkResult
			health := cr.health
			suggestedActions := cr.suggestedActions
			reason := cr.reason

			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, suggestedActions)
				if suggestedActions != nil {
					assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, suggestedActions.RepairActions)
				}
			}
		})
	}
}
