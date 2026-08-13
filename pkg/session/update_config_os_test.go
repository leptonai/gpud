package session

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	componentsos "github.com/leptonai/gpud/components/os"
)

func TestProcessUpdateConfig_OSBlockedProcessThresholds(t *testing.T) {
	t.Run("valid os config is applied", func(t *testing.T) {
		var captured componentsos.BlockedProcessThresholds
		called := false

		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				called = true
				captured = thresholds
				return nil
			},
		}

		configJSON, err := json.Marshal(componentsos.BlockedProcessThresholds{
			PersistenceThreshold: 3,
			NameRegexes:          []string{"^nvidia", "^myapp"},
		})
		require.NoError(t, err)

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: string(configJSON)}, resp)

		assert.Empty(t, resp.Error)
		require.True(t, called, "setDefaultOSBlockedProcessThresholdsFunc must be called for os config")
		assert.Equal(t, 3, captured.PersistenceThreshold)
		assert.Equal(t, []string{"^nvidia", "^myapp"}, captured.NameRegexes)
	})

	t.Run("invalid json returns error and does not call setter", func(t *testing.T) {
		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				t.Error("setter must not be called for invalid JSON")
				return nil
			},
		}

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: "{invalid json"}, resp)

		assert.NotEmpty(t, resp.Error)
	})

	t.Run("setter error is surfaced", func(t *testing.T) {
		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				return errors.New("invalid blocked process name regex")
			},
		}

		configJSON, err := json.Marshal(componentsos.BlockedProcessThresholds{
			PersistenceThreshold: 1,
			NameRegexes:          []string{"(("},
		})
		require.NoError(t, err)

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: string(configJSON)}, resp)

		assert.Contains(t, resp.Error, "invalid blocked process name regex")
	})

	t.Run("nil setter does not panic", func(t *testing.T) {
		s := &Session{}

		configJSON, err := json.Marshal(componentsos.BlockedProcessThresholds{
			PersistenceThreshold: 3,
			NameRegexes:          []string{"^nvidia"},
		})
		require.NoError(t, err)

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: string(configJSON)}, resp)

		assert.Empty(t, resp.Error)
	})

	t.Run("empty os config object disables the check", func(t *testing.T) {
		var captured componentsos.BlockedProcessThresholds
		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				captured = thresholds
				return nil
			},
		}

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: "{}"}, resp)

		assert.Empty(t, resp.Error)
		assert.True(t, captured.IsZero(), "explicit empty os config must disable D-state checking")
	})
}

func TestProcessUpdateConfig_OSFallbackRestoresStartupThresholds(t *testing.T) {
	startup := componentsos.BlockedProcessThresholds{
		PersistenceThreshold: 5,
		NameRegexes:          []string{"^nvidia"},
	}
	original := componentsos.GetStartupBlockedProcessThresholds()
	componentsos.SetStartupBlockedProcessThresholds(startup)
	t.Cleanup(func() {
		componentsos.SetStartupBlockedProcessThresholds(original)
	})

	t.Run("config without os restores startup-resolved thresholds", func(t *testing.T) {
		var captured componentsos.BlockedProcessThresholds
		called := false
		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				called = true
				captured = thresholds
				return nil
			},
		}

		// a config push for an unrelated component must not silently disable
		// D-state tracking on GPU machines: the fallback restores the
		// startup-resolved (GPU-aware) thresholds
		resp := &Response{}
		s.processUpdateConfig(map[string]string{"unsupported-component": "{}"}, resp)

		assert.Empty(t, resp.Error)
		require.True(t, called, "fallback must restore startup thresholds when os is not configured")
		assert.Equal(t, startup.PersistenceThreshold, captured.PersistenceThreshold)
		assert.Equal(t, startup.NameRegexes, captured.NameRegexes)
	})

	t.Run("config with os does not trigger fallback", func(t *testing.T) {
		var captured []componentsos.BlockedProcessThresholds
		s := &Session{
			setDefaultOSBlockedProcessThresholdsFunc: func(thresholds componentsos.BlockedProcessThresholds) error {
				captured = append(captured, thresholds)
				return nil
			},
		}

		configJSON, err := json.Marshal(componentsos.BlockedProcessThresholds{
			PersistenceThreshold: 7,
			NameRegexes:          []string{"^custom"},
		})
		require.NoError(t, err)

		resp := &Response{}
		s.processUpdateConfig(map[string]string{componentsos.Name: string(configJSON)}, resp)

		assert.Empty(t, resp.Error)
		require.Len(t, captured, 1, "setter must be called exactly once (no fallback override)")
		assert.Equal(t, 7, captured[0].PersistenceThreshold)
		assert.Equal(t, []string{"^custom"}, captured[0].NameRegexes)
	})
}
