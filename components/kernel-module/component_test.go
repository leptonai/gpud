package kernelmodule

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestNew(t *testing.T) {
	modulesToCheck := []string{"module1", "module2"}
	c := New(modulesToCheck).(*component)
	assert.Equal(t, modulesToCheck, c.modulesToCheck)
	assert.NotNil(t, c.getAllModules)
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
			c.getAllModules = func() ([]string, error) {
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
			c.getAllModules = func() ([]string, error) {
				return tt.modulesToLoad, tt.loadError
			}

			c.CheckOnce()

			states, err := c.States(context.Background())
			require.NoError(t, err)
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.wantHealthy, state.Healthy)
			if tt.wantHealthy {
				assert.Equal(t, components.StateHealthy, state.Health)
			} else {
				assert.Equal(t, components.StateUnhealthy, state.Health)
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

func TestMetrics(t *testing.T) {
	c := New(nil)
	metrics, err := c.Metrics(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestClose(t *testing.T) {
	c := New(nil)
	err := c.Close()
	assert.NoError(t, err)
}

func TestDataGetReason(t *testing.T) {
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
			wantReason:     "no module data",
		},
		{
			name:           "with error",
			data:           &Data{err: assert.AnError},
			modulesToCheck: []string{"module1"},
			wantReason:     "failed to read modules -- assert.AnError general error for testing",
		},
		{
			name:           "no modules to check",
			data:           &Data{loadedModules: map[string]struct{}{"module1": {}}},
			modulesToCheck: nil,
			wantReason:     "no modules to check",
		},
		{
			name: "all modules present",
			data: &Data{
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
				LoadedModules: []string{"module1", "module2"},
			},
			modulesToCheck: []string{"module1", "module2"},
			wantReason:     "all modules are loaded",
		},
		{
			name: "missing modules",
			data: &Data{
				loadedModules: map[string]struct{}{"module1": {}},
				LoadedModules: []string{"module1"},
			},
			modulesToCheck: []string{"module1", "module2"},
			wantReason:     `missing modules: ["module2"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := tt.data.getReason(tt.modulesToCheck)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestDataGetHealth(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		modulesToCheck []string
		wantHealth     string
		wantHealthy    bool
	}{
		{
			name:           "with error",
			data:           &Data{err: assert.AnError},
			modulesToCheck: []string{"module1"},
			wantHealth:     components.StateUnhealthy,
			wantHealthy:    false,
		},
		{
			name: "no modules to check",
			data: &Data{
				LoadedModules: []string{},
				loadedModules: map[string]struct{}{},
			},
			modulesToCheck: nil,
			wantHealth:     components.StateHealthy,
			wantHealthy:    true,
		},
		{
			name: "all modules present",
			data: &Data{
				LoadedModules: []string{"module1", "module2"},
				loadedModules: map[string]struct{}{"module1": {}, "module2": {}},
			},
			modulesToCheck: []string{"module1", "module2"},
			wantHealth:     components.StateHealthy,
			wantHealthy:    true,
		},
		{
			name: "missing modules",
			data: &Data{
				LoadedModules: []string{"module1"},
				loadedModules: map[string]struct{}{"module1": {}},
			},
			modulesToCheck: []string{"module1", "module2"},
			wantHealth:     components.StateUnhealthy,
			wantHealthy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, healthy := tt.data.getHealth(tt.modulesToCheck)
			assert.Equal(t, tt.wantHealth, health)
			assert.Equal(t, tt.wantHealthy, healthy)
		})
	}
}
