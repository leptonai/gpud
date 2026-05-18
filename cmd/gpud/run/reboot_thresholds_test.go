package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRebootThresholds(t *testing.T) {
	xidThresholds, err := parseXIDRebootThresholds(`{"94":{"rebootThreshold":1000}}`)
	require.NoError(t, err)
	assert.Equal(t, 1000, xidThresholds[94].RebootThreshold)

	sxidThresholds, err := parseSXIDRebootThresholds(`{"11004":{"rebootThreshold":7}}`)
	require.NoError(t, err)
	assert.Equal(t, 7, sxidThresholds[11004].RebootThreshold)

	_, err = parseXIDRebootThresholds(`{"94":{"rebootThreshold":0}}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebootThreshold must be positive")

	_, err = parseSXIDRebootThresholds(`{not-valid-json}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sxid reboot thresholds")
}
