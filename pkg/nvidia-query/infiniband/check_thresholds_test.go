package infiniband

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
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
			_, matched := CheckPortsAndRate(
				parsed.IBPorts(),
				tc.expectedPhysicalStates,
				tc.expectedState,
				tc.expectedAtLeastRate,
			)
			if len(matched) != tc.expectedCount {
				t.Errorf("Expected %d cards, got %d", tc.expectedCount, len(matched))
			}
			if !reflect.DeepEqual(matched, tc.expectedPortNames) {
				t.Errorf("Expected %v, got %v", tc.expectedPortNames, matched)
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
	_, matchedActive := CheckPortsAndRate(ports, []string{"LinkUp"}, "Active", 200)
	assert.Equal(t, 1, len(matchedActive), "Should match only the Active state port")
	assert.Equal(t, "mlx5_0", matchedActive[0], "Device mlx5_0 should be matched")

	// Test with a specific state that doesn't match
	_, matchedNone := CheckPortsAndRate(ports, []string{"LinkUp"}, "Init", 200)
	assert.Equal(t, 0, len(matchedNone), "Should not match any port")
}
