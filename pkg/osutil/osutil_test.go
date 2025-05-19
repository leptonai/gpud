package osutil

import (
	"os"
	"testing"
)

func TestRequireRoot(t *testing.T) {
	// We can't modify os.Geteuid, but we can test the error message
	// when not running as root
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	err := RequireRoot()
	if err == nil {
		t.Error("Expected error when not running as root, got nil")
	}
	if err.Error() != "this command needs to be run as root" {
		t.Errorf("Unexpected error message: %v", err)
	}
}
