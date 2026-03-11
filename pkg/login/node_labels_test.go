package login

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNodeLabelsJSON(t *testing.T) {
	t.Run("parses object", func(t *testing.T) {
		labels, err := ParseNodeLabelsJSON(`{"team":"ml","rack":"r42"}`)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"team": "ml", "rack": "r42"}, labels)
	})

	t.Run("supports explicit clear", func(t *testing.T) {
		labels, err := ParseNodeLabelsJSON(`{}`)
		require.NoError(t, err)
		require.NotNil(t, labels)
		assert.Empty(t, labels)
	})

	t.Run("rejects null", func(t *testing.T) {
		labels, err := ParseNodeLabelsJSON(`null`)
		require.Error(t, err)
		assert.Nil(t, labels)
		assert.Contains(t, err.Error(), "use {} to clear labels")
	})
}

func TestValidateNodeLabels(t *testing.T) {
	t.Run("accepts valid labels", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"team":   "ml",
			"rack_1": "r42",
			"zone.1": "",
		})
		require.NoError(t, err)
	})

	t.Run("rejects prefixed key", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"example.com/team": "ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be unprefixed")
	})

	t.Run("rejects invalid value", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"team": "-ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid node label value")
	})

	t.Run("rejects too many labels", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"k1": "v1",
			"k2": "v2",
			"k3": "v3",
			"k4": "v4",
			"k5": "v5",
			"k6": "v6",
			"k7": "v7",
			"k8": "v8",
			"k9": "v9",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 8 node labels")
	})
}

func TestCanonicalNodeLabels(t *testing.T) {
	canonical, err := canonicalNodeLabels(map[string]string{"rack": "r42", "team": "ml"})
	require.NoError(t, err)
	assert.Equal(t, `{"rack":"r42","team":"ml"}`, canonical)

	canonical, err = canonicalNodeLabels(map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, `{}`, canonical)

	canonical, err = canonicalNodeLabels(nil)
	require.NoError(t, err)
	assert.Empty(t, canonical)
}
