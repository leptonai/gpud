package login

import (
	"strings"
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

	t.Run("rejects empty input", func(t *testing.T) {
		labels, err := ParseNodeLabelsJSON(`   `)
		require.Error(t, err)
		assert.Nil(t, labels)
		assert.Contains(t, err.Error(), "must be a JSON object")
	})
}

func TestValidateNodeLabels(t *testing.T) {
	t.Run("accepts valid labels", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"user.node.lepton.ai/team":   "ml",
			"user.node.lepton.ai/rack_1": "r42",
			"user.node.lepton.ai/zone.1": "",
		})
		require.NoError(t, err)
	})

	t.Run("rejects invalid qualified key", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"user.node.lepton.ai/example.com/team": "ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid node label key")
	})

	t.Run("rejects invalid value", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"user.node.lepton.ai/team": "-ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid node label value")
	})

	t.Run("rejects too many labels", func(t *testing.T) {
		err := ValidateNodeLabels(map[string]string{
			"user.node.lepton.ai/k1": "v1",
			"user.node.lepton.ai/k2": "v2",
			"user.node.lepton.ai/k3": "v3",
			"user.node.lepton.ai/k4": "v4",
			"user.node.lepton.ai/k5": "v5",
			"user.node.lepton.ai/k6": "v6",
			"user.node.lepton.ai/k7": "v7",
			"user.node.lepton.ai/k8": "v8",
			"user.node.lepton.ai/k9": "v9",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 8 node labels")
	})
}

func TestNormalizeNodeLabels(t *testing.T) {
	t.Run("prefixes unprefixed keys before validation", func(t *testing.T) {
		normalized, err := normalizeNodeLabels(map[string]string{
			"team": "ml",
			"rack": "r42",
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"user.node.lepton.ai/team": "ml",
			"user.node.lepton.ai/rack": "r42",
		}, normalized)
	})

	t.Run("preserves managed prefix", func(t *testing.T) {
		normalized, err := normalizeNodeLabels(map[string]string{
			"user.node.lepton.ai/team": "ml",
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"user.node.lepton.ai/team": "ml",
		}, normalized)
	})

	t.Run("rejects duplicate managed keys after normalization", func(t *testing.T) {
		_, err := normalizeNodeLabels(map[string]string{
			"team":                     "ml",
			"user.node.lepton.ai/team": "platform",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "normalize to the same managed key")
	})

	t.Run("rejects invalid final key after prefixing", func(t *testing.T) {
		_, err := normalizeNodeLabels(map[string]string{
			"example.com/team": "ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid node label key")
	})

	t.Run("accepts max kubernetes final label-name length after prefixing", func(t *testing.T) {
		key := strings.Repeat("a", 63)
		normalized, err := normalizeNodeLabels(map[string]string{
			key: "ml",
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			managedNodeLabelPrefix + key: "ml",
		}, normalized)
	})

	t.Run("rejects final label name longer than kubernetes limit after prefixing", func(t *testing.T) {
		key := strings.Repeat("a", 64)
		_, err := normalizeNodeLabels(map[string]string{
			key: "ml",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid node label key")
	})
}

func TestCanonicalNodeLabels(t *testing.T) {
	canonical, err := canonicalNodeLabels(map[string]string{"rack": "r42", "team": "ml"})
	require.NoError(t, err)
	assert.Equal(t, `{"user.node.lepton.ai/rack":"r42","user.node.lepton.ai/team":"ml"}`, canonical)

	canonical, err = canonicalNodeLabels(map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, `{}`, canonical)

	canonical, err = canonicalNodeLabels(nil)
	require.NoError(t, err)
	assert.Empty(t, canonical)

	canonical, err = canonicalNodeLabels(map[string]string{"example.com/team": "ml"})
	require.Error(t, err)
	assert.Empty(t, canonical)
}
