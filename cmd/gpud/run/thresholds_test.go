package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseThresholds(t *testing.T) {
	xidThresholds, err := parseXIDThresholds(`{"overrides":{"94":{"rebootThreshold":1000}}}`)
	require.NoError(t, err)
	assert.Equal(t, 1000, xidThresholds.Overrides[94].RebootThreshold)

	sxidThresholds, err := parseSXIDThresholds(`{"overrides":{"11004":{"rebootThreshold":7}}}`)
	require.NoError(t, err)
	assert.Equal(t, 7, sxidThresholds.Overrides[11004].RebootThreshold)

	_, err = parseXIDThresholds(`{"overrides":{"94":{"rebootThreshold":0}}}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebootThreshold must be positive")

	_, err = parseXIDThresholds(`{"94":{"rebootThreshold":1000}}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "94"`)

	_, err = parseSXIDThresholds(`{not-valid-json}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sxid thresholds")
}
