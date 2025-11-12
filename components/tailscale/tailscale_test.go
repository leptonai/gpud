package tailscale

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTailscaleVersion(t *testing.T) {
	output := `1.80.0

	tailscale commit: abc
	other commit: def`
	version, err := parseTailscaleVersion(output)
	assert.NoError(t, err)
	assert.Equal(t, "1.80.0", version)
}

func TestParseTailscaledVersion(t *testing.T) {
	output := `v1.80.0

	tailscale commit: abc`
	version, err := parseTailscaledVersion(output)
	assert.NoError(t, err)
	assert.Equal(t, "v1.80.0", version)
}

func TestParseTailscaleVersionInvalid(t *testing.T) {
	_, err := parseTailscaleVersion("no version here")
	assert.Error(t, err)
}
