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
)

func TestNew(t *testing.T) {
	modulesToCheck := []string{"module1", "module2"}
	c := New(modulesToCheck).(*component)
	assert.Equal(t, modulesToCheck, c.modulesToCheck)
	assert.NotNil(t, c.getAllModulesFunc)
}

func TestComponentName(t *testing.T) {
	c := New(nil)
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
			c := New(nil).(*component)
			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			c.CheckOnce()

			c.lastMu.RLock()
			defer c.lastMu.RUnlock()

			require.NotNil(t, c.lastData)
			assert.Equal(t, tt.wantModules, c.lastData.LoadedModules)
			if tt.wantError {
				assert.Error(t, c.lastData.err)
			} else {
				assert.NoError(t, c.lastData.err)
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
			c := New(tt.modulesToCheck).(*component)
			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			c.CheckOnce()

			states, err := c.HealthStates(context.Background())
			require.NoError(t, err)
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.wantHealthy, state.DeprecatedHealthy)
			if tt.wantHealthy {
				assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
			} else {
				assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
			}
		})
	}
}

func TestEvents(t *testing.T) {
	c := New(nil)
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestClose(t *testing.T) {
	c := New(nil)
	err := c.Close()
	assert.NoError(t, err)
}

func TestGetReason(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		modulesToCheck []string
		wantReason     string
	}{
		{
			name:           "nil data",
			data:           nil,
			modulesToCheck: []string{"module1"},
			wantReason:     "", // nil data has no reason
		},
		{
			name:           "with error",
			data:           &Data{err: assert.AnError, reason: "error getting all modules: assert.AnError general error for testing"},
			modulesToCheck: []string{"module1"},
			wantReason:     "error getting all modules: assert.AnError general error for testing",
		},
		{
			name:           "no modules to check",
			data:           &Data{loadedModules: map[string]struct{}{"module1": {}}, reason: "all modules are loaded"},
			modulesToCheck: nil,
			wantReason:     "all modules are loaded",
		},
		{
			name: "all modules present",
			data: &Data{
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
				LoadedModules: []string{"module1", "module2"},
				reason:        "all modules are loaded",
				healthy:       true,
			},
			modulesToCheck: []string{"module1", "module2"},
			wantReason:     "all modules are loaded",
		},
		{
			name: "missing modules",
			data: &Data{
				loadedModules: map[string]struct{}{"module1": {}},
				LoadedModules: []string{"module1"},
				reason:        `missing modules: ["module2"]`,
				healthy:       false,
			},
			modulesToCheck: []string{"module1", "module2"},
			wantReason:     `missing modules: ["module2"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.data == nil {
				assert.Equal(t, tt.wantReason, "")
			} else {
				assert.Equal(t, tt.wantReason, tt.data.reason)
			}
		})
	}
}

func TestGetHealth(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		modulesToCheck []string
		wantHealth     apiv1.HealthStateType
		wantHealthy    bool
	}{
		{
			name:           "with error",
			data:           &Data{err: assert.AnError, healthy: false},
			modulesToCheck: []string{"module1"},
			wantHealth:     apiv1.StateTypeUnhealthy,
			wantHealthy:    false,
		},
		{
			name: "no modules to check",
			data: &Data{
				LoadedModules: []string{},
				loadedModules: map[string]struct{}{},
				healthy:       true,
			},
			modulesToCheck: nil,
			wantHealth:     apiv1.StateTypeHealthy,
			wantHealthy:    true,
		},
		{
			name: "all modules present",
			data: &Data{
				LoadedModules: []string{"module1", "module2"},
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
				healthy:       true,
			},
			modulesToCheck: []string{"module1", "module2"},
			wantHealth:     apiv1.StateTypeHealthy,
			wantHealthy:    true,
		},
		{
			name: "missing modules",
			data: &Data{
				LoadedModules: []string{"module1"},
				loadedModules: map[string]struct{}{"module1": {}},
				healthy:       false,
			},
			modulesToCheck: []string{"module1", "module2"},
			wantHealth:     apiv1.StateTypeUnhealthy,
			wantHealthy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := apiv1.StateTypeHealthy
			if tt.data != nil && !tt.data.healthy {
				health = apiv1.StateTypeUnhealthy
			}

			assert.Equal(t, tt.wantHealth, health)
			assert.Equal(t, tt.wantHealthy, tt.data != nil && tt.data.healthy)
		})
	}
}

func TestDataGetStates(t *testing.T) {
	tests := []struct {
		name        string
		data        *Data
		wantHealthy bool
		wantHealth  apiv1.HealthStateType
		wantReason  string
		wantError   bool
	}{
		{
			name:        "nil data",
			data:        nil,
			wantHealthy: true,
			wantHealth:  apiv1.StateTypeHealthy,
			wantReason:  "no data yet",
			wantError:   false,
		},
		{
			name:        "with error",
			data:        &Data{err: assert.AnError, healthy: false, reason: "error getting all modules: assert.AnError general error for testing"},
			wantHealthy: false,
			wantHealth:  apiv1.StateTypeUnhealthy,
			wantReason:  "error getting all modules: assert.AnError general error for testing",
			wantError:   true,
		},
		{
			name: "no modules to check",
			data: &Data{
				LoadedModules: []string{},
				loadedModules: map[string]struct{}{},
				healthy:       true,
				reason:        "all modules are loaded",
			},
			wantHealthy: true,
			wantHealth:  apiv1.StateTypeHealthy,
			wantReason:  "all modules are loaded",
			wantError:   false,
		},
		{
			name: "all modules present",
			data: &Data{
				LoadedModules: []string{"module1", "module2"},
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
				healthy:       true,
				reason:        "all modules are loaded",
			},
			wantHealthy: true,
			wantHealth:  apiv1.StateTypeHealthy,
			wantReason:  "all modules are loaded",
			wantError:   false,
		},
		{
			name: "missing modules",
			data: &Data{
				LoadedModules: []string{"module1"},
				loadedModules: map[string]struct{}{"module1": {}},
				healthy:       false,
				reason:        `missing modules: ["module2"]`,
			},
			wantHealthy: false,
			wantHealth:  apiv1.StateTypeUnhealthy,
			wantReason:  `missing modules: ["module2"]`,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)

			require.Len(t, states, 1)
			state := states[0]

			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.wantReason, state.Reason)
			assert.Equal(t, tt.wantHealth, state.Health)
			assert.Equal(t, tt.wantHealthy, state.DeprecatedHealthy)

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
			if tt.data != nil {
				assert.Contains(t, state.DeprecatedExtraInfo, "data")
				assert.Contains(t, state.DeprecatedExtraInfo, "encoding")
				assert.Equal(t, "json", state.DeprecatedExtraInfo["encoding"])

				// Verify that the JSON encoding works
				var decodedData map[string]interface{}
				err := json.Unmarshal([]byte(state.DeprecatedExtraInfo["data"]), &decodedData)
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
			wantReason:     "error getting all modules: module load error",
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
			c := New(tt.modulesToCheck).(*component)
			c.getAllModulesFunc = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			c.CheckOnce()

			c.lastMu.RLock()
			defer c.lastMu.RUnlock()

			require.NotNil(t, c.lastData)
			assert.Equal(t, tt.wantHealthy, c.lastData.healthy)
			assert.Equal(t, tt.wantReason, c.lastData.reason)

			if tt.loadError != nil {
				assert.Equal(t, tt.loadError, c.lastData.err)
			} else {
				assert.NoError(t, c.lastData.err)
			}
		})
	}
}

// Test that timestamp is properly set
func TestDataTimestamp(t *testing.T) {
	c := New(nil).(*component)

	beforeCheck := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	c.CheckOnce()
	time.Sleep(10 * time.Millisecond)
	afterCheck := time.Now().UTC()

	c.lastMu.RLock()
	defer c.lastMu.RUnlock()

	require.NotNil(t, c.lastData)
	assert.True(t, !c.lastData.ts.Before(beforeCheck), "Timestamp should be after the check started")
	assert.True(t, !c.lastData.ts.After(afterCheck), "Timestamp should be before the check ended")
}
