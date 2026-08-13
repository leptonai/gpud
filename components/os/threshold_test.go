//nolint:revive // package name follows the directory import path used across the codebase.
package os

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultBlockedProcessThresholds_BuiltIn(t *testing.T) {
	th := GetDefaultBlockedProcessThresholds()
	assert.Equal(t, DefaultBlockedProcessPersistenceThreshold, th.PersistenceThreshold)
	assert.Empty(t, th.NameRegexes, "built-in default must keep D-state checking disabled until auto-enabled or configured")
	assert.True(t, th.IsZero())
	assert.False(t, th.MatchesName("nvidia-smi"))
}

func TestDefaultBlockedProcessNameRegexes(t *testing.T) {
	regexes := DefaultBlockedProcessNameRegexes()
	require.Equal(t, []string{"^nvidia"}, regexes)

	// mutating the returned slice must not affect subsequent calls
	regexes[0] = "mutated"
	assert.Equal(t, []string{"^nvidia"}, DefaultBlockedProcessNameRegexes())
}

func TestSetDefaultBlockedProcessThresholds(t *testing.T) {
	original := GetDefaultBlockedProcessThresholds()
	t.Cleanup(func() {
		require.NoError(t, SetDefaultBlockedProcessThresholds(original))
	})

	// round trip
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 3,
		NameRegexes:          []string{"^nvidia", " uvm_ ", ""},
	}))
	th := GetDefaultBlockedProcessThresholds()
	assert.Equal(t, 3, th.PersistenceThreshold)
	assert.Equal(t, []string{"^nvidia", "uvm_"}, th.NameRegexes, "blank entries must be trimmed/dropped")
	assert.False(t, th.IsZero())
	assert.True(t, th.MatchesName("nvidia-smi"))
	assert.True(t, th.MatchesName("uvm_va_space"))
	assert.False(t, th.MatchesName("dd"))

	// non-positive persistence threshold resets to the default
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: -1,
		NameRegexes:          []string{"^nvidia"},
	}))
	assert.Equal(t, DefaultBlockedProcessPersistenceThreshold, GetDefaultBlockedProcessThresholds().PersistenceThreshold)

	// invalid regex rejected, previous config preserved
	require.Error(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 1,
		NameRegexes:          []string{"(("},
	}))
	th = GetDefaultBlockedProcessThresholds()
	assert.Equal(t, DefaultBlockedProcessPersistenceThreshold, th.PersistenceThreshold)
	assert.Equal(t, []string{"^nvidia"}, th.NameRegexes)

	// empty regex set disables the check
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 5,
		NameRegexes:          []string{},
	}))
	th = GetDefaultBlockedProcessThresholds()
	assert.True(t, th.IsZero())
	assert.False(t, th.MatchesName("nvidia-smi"))
}

func TestNewBlockedProcessThresholds(t *testing.T) {
	th, err := newBlockedProcessThresholds(0, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultBlockedProcessPersistenceThreshold, th.PersistenceThreshold)
	assert.True(t, th.IsZero())

	th, err = newBlockedProcessThresholds(1, []string{"^nvidia"})
	require.NoError(t, err)
	assert.Equal(t, 1, th.PersistenceThreshold)
	assert.True(t, th.MatchesName("nvidia-smi"))

	_, err = newBlockedProcessThresholds(1, []string{"(("})
	require.Error(t, err)
}

func TestStartupBlockedProcessThresholds(t *testing.T) {
	originalDefault := GetDefaultBlockedProcessThresholds()
	t.Cleanup(func() {
		require.NoError(t, SetDefaultBlockedProcessThresholds(originalDefault))
		startupBlockedProcessThresholdsMu.Lock()
		startupBlockedProcessThresholds = nil
		startupBlockedProcessThresholdsMu.Unlock()
	})

	// never recorded in this test binary order-independently: falls back to
	// the current defaults so the updateConfig fallback leaves them unchanged
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 2,
		NameRegexes:          []string{"^dd$"},
	}))
	startup := GetStartupBlockedProcessThresholds()
	assert.Equal(t, 2, startup.PersistenceThreshold)
	assert.Equal(t, []string{"^dd$"}, startup.NameRegexes)

	// record a startup baseline, then change the runtime defaults (e.g., via
	// updateConfig): the baseline must stay at the startup-resolved value
	SetStartupBlockedProcessThresholds(GetDefaultBlockedProcessThresholds())
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 9,
		NameRegexes:          []string{"^other$"},
	}))
	startup = GetStartupBlockedProcessThresholds()
	assert.Equal(t, 2, startup.PersistenceThreshold)
	assert.Equal(t, []string{"^dd$"}, startup.NameRegexes)

	// mutating the returned slice must not affect the recorded baseline
	startup.NameRegexes[0] = "mutated"
	assert.Equal(t, []string{"^dd$"}, GetStartupBlockedProcessThresholds().NameRegexes)
}
