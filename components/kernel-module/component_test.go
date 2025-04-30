package kernelmodule

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

func TestComponentName(t *testing.T) {
	c, err := New(&components.GPUdInstance{})
	require.NoError(t, err)
	assert.Equal(t, Name, c.Name())
}

func TestCheckOnce(t *testing.T) {
	tests := []struct {
		name          string
		modulesToLoad []string
		loadError     error
		wantModules   []string
		wantError     bool
	}{
		{
			name:          "successful load",
			modulesToLoad: []string{"module1", "module2"},
			loadError:     nil,
			wantModules:   []string{"module1", "module2"},
			wantError:     false,
		},
		{
			name:          "load with error",
			modulesToLoad: nil,
			loadError:     assert.AnError,
			wantModules:   nil,
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := New(&components.GPUdInstance{})
			require.NoError(t, err)
			c := comp.(*component)

			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			_ = c.Check()

			c.lastMu.RLock()
			defer c.lastMu.RUnlock()

			require.NotNil(t, c.lastCheckResult)
			assert.Equal(t, tt.wantModules, c.lastCheckResult.LoadedModules)
			if tt.wantError {
				assert.Error(t, c.lastCheckResult.err)
			} else {
				assert.NoError(t, c.lastCheckResult.err)
			}
		})
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		name           string
		modulesToLoad  []string
		modulesToCheck []string
		loadError      error
		wantHealthy    bool
	}{
		{
			name:           "all modules present",
			modulesToLoad:  []string{"module1", "module2"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantHealthy:    true,
		},
		{
			name:           "missing modules",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantHealthy:    false,
		},
		{
			name:           "load error",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      assert.AnError,
			wantHealthy:    false,
		},
		{
			name:           "no modules to check",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: nil,
			loadError:      nil,
			wantHealthy:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
			require.NoError(t, err)
			c := comp.(*component)

			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			_ = c.Check()

			states := c.LastHealthStates()
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			if tt.wantHealthy {
				assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
			} else {
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
			}
		})
	}
}

func TestEvents(t *testing.T) {
	c, err := New(&components.GPUdInstance{})
	require.NoError(t, err)
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestClose(t *testing.T) {
	c, err := New(&components.GPUdInstance{})
	require.NoError(t, err)
	err = c.Close()
	assert.NoError(t, err)
}

func TestGetReason(t *testing.T) {
	tests := []struct {
		name           string
		modulesToLoad  []string
		modulesToCheck []string
		loadError      error
		wantReason     string
	}{
		{
			name:           "nil data",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      nil,
			wantReason:     "no data yet",
		},
		{
			name:           "with error",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      assert.AnError,
			wantReason:     "error getting all modules",
		},
		{
			name:           "no modules to check",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: nil,
			loadError:      nil,
			wantReason:     "all modules are loaded",
		},
		{
			name:           "all modules present",
			modulesToLoad:  []string{"module1", "module2"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantReason:     "all modules are loaded",
		},
		{
			name:           "missing modules",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantReason:     `missing modules: ["module2"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil data" {
				// Special case for nil data test
				comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
				require.NoError(t, err)
				c := comp.(*component)

				// Ensure lastCheckResult is nil
				c.lastMu.Lock()
				c.lastCheckResult = nil
				c.lastMu.Unlock()

				states := c.LastHealthStates()
				require.Len(t, states, 1)
				assert.Equal(t, tt.wantReason, states[0].Reason)
				return
			}

			// For all other tests, create component and run Check
			comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
			require.NoError(t, err)
			c := comp.(*component)

			// Mock the getAllModulesFunc
			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			// Run Check to initialize the data
			c.Check()

			// Get health states which will have the computed reason
			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tt.wantReason, states[0].Reason)
		})
	}
}

func TestGetHealth(t *testing.T) {
	tests := []struct {
		name           string
		modulesToLoad  []string
		modulesToCheck []string
		loadError      error
		wantHealth     apiv1.HealthStateType
		wantHealthy    bool
	}{
		{
			name:           "nil data",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      nil,
			wantHealth:     apiv1.HealthStateTypeHealthy,
			wantHealthy:    true,
		},
		{
			name:           "with error",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      assert.AnError,
			wantHealth:     apiv1.HealthStateTypeUnhealthy,
			wantHealthy:    false,
		},
		{
			name:           "no modules to check",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: nil,
			loadError:      nil,
			wantHealth:     apiv1.HealthStateTypeHealthy,
			wantHealthy:    true,
		},
		{
			name:           "all modules present",
			modulesToLoad:  []string{"module1", "module2"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantHealth:     apiv1.HealthStateTypeHealthy,
			wantHealthy:    true,
		},
		{
			name:           "missing modules",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantHealth:     apiv1.HealthStateTypeUnhealthy,
			wantHealthy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil data" {
				// Special case for nil data test
				comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
				require.NoError(t, err)
				c := comp.(*component)

				// Ensure lastCheckResult is nil
				c.lastMu.Lock()
				c.lastCheckResult = nil
				c.lastMu.Unlock()

				states := c.LastHealthStates()
				require.Len(t, states, 1)
				assert.Equal(t, tt.wantHealth, states[0].Health)
				assert.Equal(t, tt.wantHealthy, states[0].Health == apiv1.HealthStateTypeHealthy)
				return
			}

			// For all other tests, create component and run Check
			comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
			require.NoError(t, err)
			c := comp.(*component)

			// Mock the getAllModulesFunc
			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			// Run Check to initialize the data
			c.Check()

			// Get health states which will have the computed health
			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tt.wantHealth, states[0].Health)
			assert.Equal(t, tt.wantHealthy, states[0].Health == apiv1.HealthStateTypeHealthy)
		})
	}
}

func TestDataGetStates(t *testing.T) {
	tests := []struct {
		name        string
		data        *checkResult
		wantHealthy bool
		wantHealth  apiv1.HealthStateType
		wantReason  string
		wantError   bool
	}{
		{
			name:        "nil data",
			data:        nil,
			wantHealthy: true,
			wantHealth:  apiv1.HealthStateTypeHealthy,
			wantReason:  "no data yet",
			wantError:   false,
		},
		{
			name:        "with error",
			data:        &checkResult{err: assert.AnError, health: apiv1.HealthStateTypeUnhealthy, reason: "error getting all modules"},
			wantHealthy: false,
			wantHealth:  apiv1.HealthStateTypeUnhealthy,
			wantReason:  "error getting all modules",
			wantError:   true,
		},
		{
			name: "no modules to check",
			data: &checkResult{
				LoadedModules: []string{"a", "b"},
				loadedModules: map[string]struct{}{"a": {}, "b": {}},
				health:        apiv1.HealthStateTypeHealthy,
				reason:        "all modules are loaded",
			},
			wantHealthy: true,
			wantHealth:  apiv1.HealthStateTypeHealthy,
			wantReason:  "all modules are loaded",
			wantError:   false,
		},
		{
			name: "all modules present",
			data: &checkResult{
				LoadedModules: []string{"module1", "module2"},
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
				health:        apiv1.HealthStateTypeHealthy,
				reason:        "all modules are loaded",
			},
			wantHealthy: true,
			wantHealth:  apiv1.HealthStateTypeHealthy,
			wantReason:  "all modules are loaded",
			wantError:   false,
		},
		{
			name: "missing modules",
			data: &checkResult{
				LoadedModules: []string{"module1"},
				loadedModules: map[string]struct{}{"module1": {}},
				health:        apiv1.HealthStateTypeUnhealthy,
				reason:        `missing modules: ["module2"]`,
			},
			wantHealthy: false,
			wantHealth:  apiv1.HealthStateTypeUnhealthy,
			wantReason:  `missing modules: ["module2"]`,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.HealthStates()

			require.Len(t, states, 1)
			state := states[0]

			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.wantReason, state.Reason)
			assert.Equal(t, tt.wantHealth, state.Health)

			// Check Error field is set correctly
			if tt.wantError {
				assert.NotEmpty(t, state.Error, "Error should be set when there's an error")
				if tt.data != nil && tt.data.err != nil {
					assert.Equal(t, tt.data.err.Error(), state.Error, "Error should match Data.err")
				}
			} else {
				assert.Empty(t, state.Error, "Error should be empty when there's no error")
			}

			// Check that ExtraInfo exists for non-nil data
			if tt.data != nil && len(tt.data.LoadedModules) > 0 {
				assert.Contains(t, state.ExtraInfo, "data")

				// Verify that the JSON encoding works
				var decodedData map[string]interface{}
				err := json.Unmarshal([]byte(state.ExtraInfo["data"]), &decodedData)
				assert.NoError(t, err, "Should be able to decode the JSON data")

				if len(tt.data.LoadedModules) > 0 {
					assert.Contains(t, decodedData, "loaded_modules", "Should contain loaded_modules field")
				}
			}
		})
	}
}

// Add additional test for component CheckOnce implementation
func TestCheckOnceLogic(t *testing.T) {
	tests := []struct {
		name           string
		modulesToLoad  []string
		modulesToCheck []string
		loadError      error
		wantHealthy    bool
		wantReason     string
	}{
		{
			name:           "load error",
			modulesToLoad:  nil,
			modulesToCheck: []string{"module1"},
			loadError:      fmt.Errorf("module load error"),
			wantHealthy:    false,
			wantReason:     "error getting all modules",
		},
		{
			name:           "all required modules loaded",
			modulesToLoad:  []string{"module1", "module2", "module3"},
			modulesToCheck: []string{"module1", "module2"},
			loadError:      nil,
			wantHealthy:    true,
			wantReason:     "all modules are loaded",
		},
		{
			name:           "missing required modules",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: []string{"module1", "module2", "module3"},
			loadError:      nil,
			wantHealthy:    false,
			wantReason:     `missing modules: ["module2" "module3"]`,
		},
		{
			name:           "no modules to check",
			modulesToLoad:  []string{"module1"},
			modulesToCheck: nil,
			loadError:      nil,
			wantHealthy:    true,
			wantReason:     "all modules are loaded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := New(&components.GPUdInstance{KernelModulesToCheck: tt.modulesToCheck})
			require.NoError(t, err)
			c := comp.(*component)

			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			_ = c.Check()

			c.lastMu.RLock()
			defer c.lastMu.RUnlock()

			require.NotNil(t, c.lastCheckResult)
			assert.Equal(t, tt.wantHealthy, c.lastCheckResult.health == apiv1.HealthStateTypeHealthy)
			assert.Equal(t, tt.wantReason, c.lastCheckResult.reason)

			if tt.loadError != nil {
				assert.Equal(t, tt.loadError, c.lastCheckResult.err)
			} else {
				assert.NoError(t, c.lastCheckResult.err)
			}
		})
	}
}

// Test that timestamp is properly set
func TestDataTimestamp(t *testing.T) {
	comp, err := New(&components.GPUdInstance{})
	require.NoError(t, err)
	c := comp.(*component)

	beforeCheck := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	_ = c.Check()
	time.Sleep(10 * time.Millisecond)
	afterCheck := time.Now().UTC()

	c.lastMu.RLock()
	defer c.lastMu.RUnlock()

	require.NotNil(t, c.lastCheckResult)
	assert.True(t, !c.lastCheckResult.ts.Before(beforeCheck), "Timestamp should be after the check started")
	assert.True(t, !c.lastCheckResult.ts.After(afterCheck), "Timestamp should be before the check ended")
}
