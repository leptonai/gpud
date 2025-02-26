package netutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicIP(t *testing.T) {
	if os.Getenv("GPUD_TEST_PUBLIC_IPS") == "true" {
		t.Skip("Skipping PublicIP test")
	}

	ip, err := PublicIP()
	if err != nil {
		t.Logf("PublicIP test skipped due to error: %v", err)
		t.Skip("PublicIP test requires network access and curl")
	}

	assert.NotEmpty(t, ip, "PublicIP should return a non-empty string")
}
