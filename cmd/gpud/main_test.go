package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_JSONCommandErrorWritesJSONOnly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{"gpud", "machine-info", "--output-format", "json", "--log-level", "invalid-level"},
		&stdout,
		&stderr,
	)

	require.Equal(t, 1, exitCode)
	assert.Empty(t, stderr.String())

	var payload map[string]map[string]string
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Contains(t, payload, "error")
	assert.Equal(t, "invalid_log_level", payload["error"]["code"])
	assert.NotEmpty(t, payload["error"]["message"])
}

func TestRun_NonJSONErrorWritesStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{"gpud", "machine-info", "--log-level", "invalid-level"},
		&stdout,
		&stderr,
	)

	require.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid-level")
}
