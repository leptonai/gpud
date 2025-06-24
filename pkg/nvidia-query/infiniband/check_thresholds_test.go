package infiniband

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPortsAndRate(t *testing.T) {
	tt := []struct {
		fileName               string
		expectedPhysicalStates []string
		expectedState          string
		expectedAtLeastRate    int
		expectedCount          int
		expectedPortNames      []string
	}{
		{
			fileName:               "testdata/ibstat.47.0.a100.all.active.0",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    200,
			expectedCount:          9,
			expectedPortNames:      []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8"},
		},
		{
			fileName:               "testdata/ibstat.47.0.a100.all.active.0",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    100,
			expectedCount:          9,
			expectedPortNames:      []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.all.active.0",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    400,
			expectedCount:          8,
			expectedPortNames:      []string{"mlx5_0", "mlx5_10", "mlx5_11", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_9"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.all.active.1",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    400,
			expectedCount:          8,
			expectedPortNames:      []string{"mlx5_0", "mlx5_10", "mlx5_11", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_9"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.all.active.2",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    400,
			expectedCount:          8,
			expectedPortNames:      []string{"mlx5_0", "mlx5_1", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7", "mlx5_8", "mlx5_9"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.some.down.0",
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedAtLeastRate:    400,
			expectedCount:          8,
			expectedPortNames:      []string{"mlx5_0", "mlx5_10", "mlx5_11", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_9"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.some.down.1",
			expectedAtLeastRate:    400,
			expectedPhysicalStates: []string{"LinkUp"},
			expectedState:          "Active",
			expectedCount:          6,
			expectedPortNames:      []string{"mlx5_0", "mlx5_10", "mlx5_3", "mlx5_4", "mlx5_6", "mlx5_9"},
		},
		{
			fileName:               "testdata/ibstat.47.0.h100.some.down.with.polling.1",
			expectedPhysicalStates: []string{"Disabled", "Polling"},
			expectedState:          "",
			expectedAtLeastRate:    0,
			expectedCount:          2,
			expectedPortNames:      []string{"mlx5_11", "mlx5_5"},
		},
	}
	for _, tc := range tt {
		t.Run(tc.fileName, func(t *testing.T) {
			content, err := os.ReadFile(tc.fileName)
			if err != nil {
				t.Fatalf("Failed to read file %s: %v", tc.fileName, err)
			}
			parsed, err := ParseIBStat(string(content))
			if err != nil {
				t.Fatalf("Failed to parse ibstat file %s: %v", tc.fileName, err)
			}
			matched := checkPortsAndRate(
				parsed.IBPorts(),
				tc.expectedPhysicalStates,
				tc.expectedState,
				tc.expectedAtLeastRate,
			)
			if len(matched) != tc.expectedCount {
				t.Errorf("Expected %d cards, got %d", tc.expectedCount, len(matched))
			}
			// Extract device names from matched ports
			var matchedNames []string
			for _, port := range matched {
				matchedNames = append(matchedNames, port.Device)
			}
			if !reflect.DeepEqual(matchedNames, tc.expectedPortNames) {
				t.Errorf("Expected %v, got %v", tc.expectedPortNames, matchedNames)
			}
		})
	}
}

func TestCheckPortsAndRateWithState(t *testing.T) {
	ports := []IBPort{
		{
			Device:        "mlx5_0",
			State:         "Active",
			PhysicalState: "LinkUp",
			Rate:          200,
		},
		{
			Device:        "mlx5_1",
			State:         "Down",
			PhysicalState: "LinkUp",
			Rate:          200,
		},
	}

	// Test with a specific state
	matchedActive := checkPortsAndRate(ports, []string{"LinkUp"}, "Active", 200)
	assert.Equal(t, 1, len(matchedActive), "Should match only the Active state port")
	assert.Equal(t, "mlx5_0", matchedActive[0].Device, "Device mlx5_0 should be matched")

	// Test with a specific state that doesn't match
	matchedNone := checkPortsAndRate(ports, []string{"LinkUp"}, "Init", 200)
	assert.Equal(t, 0, len(matchedNone), "Should not match any port")
}

func TestEvaluatePortsAndRate(t *testing.T) {
	tests := []struct {
		name                 string
		allPorts             []IBPort
		atLeastPorts         int
		atLeastRate          int
		expectError          bool
		expectedErrorMsg     string
		expectedProblemCount int
	}{
		{
			name: "all ports meet thresholds",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 400},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", Rate: 400},
			},
			atLeastPorts: 2,
			atLeastRate:  400,
			expectError:  false,
		},
		{
			name: "zero thresholds",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 400},
			},
			atLeastPorts: 0,
			atLeastRate:  0,
			expectError:  false,
		},
		{
			name: "insufficient ports with required rate",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 200},
				{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", Rate: 200},
			},
			atLeastPorts:     2,
			atLeastRate:      400,
			expectError:      true,
			expectedErrorMsg: "only 0 port(s) are active and >=400 Gb/s, expect >=2 port(s)",
		},
		{
			name: "some ports disabled",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 400},
				{Device: "mlx5_1", State: "Down", PhysicalState: "Disabled", Rate: 400},
			},
			atLeastPorts:         2,
			atLeastRate:          400,
			expectError:          true,
			expectedErrorMsg:     "only 1 port(s) are active and >=400 Gb/s, expect >=2 port(s); 1 device(s) found Disabled (mlx5_1)",
			expectedProblemCount: 1,
		},
		{
			name: "some ports polling",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 400},
				{Device: "mlx5_1", State: "Init", PhysicalState: "Polling", Rate: 400},
			},
			atLeastPorts:         2,
			atLeastRate:          400,
			expectError:          true,
			expectedErrorMsg:     "only 1 port(s) are active and >=400 Gb/s, expect >=2 port(s); 1 device(s) found Polling (mlx5_1)",
			expectedProblemCount: 1,
		},
		{
			name: "mixed disabled and polling",
			allPorts: []IBPort{
				{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 400},
				{Device: "mlx5_1", State: "Down", PhysicalState: "Disabled", Rate: 400},
				{Device: "mlx5_2", State: "Init", PhysicalState: "Polling", Rate: 400},
			},
			atLeastPorts:         3,
			atLeastRate:          400,
			expectError:          true,
			expectedErrorMsg:     "only 1 port(s) are active and >=400 Gb/s, expect >=3 port(s); 1 device(s) found Disabled (mlx5_1); 1 device(s) found Polling (mlx5_2)",
			expectedProblemCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problematicPorts, err := EvaluatePortsAndRate(tt.allPorts, tt.atLeastPorts, tt.atLeastRate)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErrorMsg, err.Error())
				assert.Equal(t, tt.expectedProblemCount, len(problematicPorts))
			} else {
				assert.NoError(t, err)
				assert.Nil(t, problematicPorts)
			}
		})
	}
}

// Test with real testdata files
func TestEvaluatePortsAndRateWithTestdata(t *testing.T) {
	tests := []struct {
		fileName         string
		atLeastPorts     int
		atLeastRate      int
		expectError      bool
		problemPortCount int
	}{
		{
			fileName:     "testdata/ibstat.47.0.a100.all.active.0",
			atLeastPorts: 9,
			atLeastRate:  200,
			expectError:  false,
		},
		{
			fileName:         "testdata/ibstat.47.0.a100.all.active.0",
			atLeastPorts:     10,
			atLeastRate:      200,
			expectError:      true,
			problemPortCount: 0, // All ports are up, just not enough of them
		},
		{
			fileName:     "testdata/ibstat.47.0.h100.all.active.0",
			atLeastPorts: 8,
			atLeastRate:  400,
			expectError:  false,
		},
		{
			fileName:         "testdata/ibstat.47.0.h100.some.down.0",
			atLeastPorts:     10,
			atLeastRate:      400,
			expectError:      true,
			problemPortCount: 3, // 3 ports are disabled (mlx5_2, mlx5_7, mlx5_8)
		},
		{
			fileName:         "testdata/ibstat.47.0.h100.some.down.1",
			atLeastPorts:     8,
			atLeastRate:      400,
			expectError:      true,
			problemPortCount: 2, // 2 ports are disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			content, err := os.ReadFile(tt.fileName)
			require.NoError(t, err)

			parsed, err := ParseIBStat(string(content))
			require.NoError(t, err)

			problematicPorts, err := EvaluatePortsAndRate(parsed.IBPorts(), tt.atLeastPorts, tt.atLeastRate)

			if tt.expectError {
				assert.Error(t, err)
				if tt.problemPortCount > 0 {
					assert.Equal(t, tt.problemPortCount, len(problematicPorts))
				}
			} else {
				assert.NoError(t, err)
				assert.Nil(t, problematicPorts)
			}
		})
	}
}
