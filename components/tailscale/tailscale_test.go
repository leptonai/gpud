package tailscale

import (
	"testing"
)

func TestCheckTailscaledInstalled(t *testing.T) {
	t.Logf("tailscaled installed: %v", checkTailscaledInstalled())
}
