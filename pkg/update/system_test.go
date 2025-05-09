package update

import (
	"errors"
	"os"
	"runtime"
	"testing"

	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
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

func TestDetectUbuntuVersion(t *testing.T) {
	// This test only verifies the function doesn't panic
	// We can't mock exec.Command without external libraries
	if runtime.GOOS != "linux" {
		t.Skip("Test only applicable on Linux")
	}

	version := detectUbuntuVersion()
	t.Logf("Detected Ubuntu version: %q", version)
}

func TestSystemdFunctions(t *testing.T) {
	// Skip if systemctl doesn't exist
	if !pkd_systemd.SystemctlExists() {
		t.Skip("systemctl not available")
	}

	// These tests just verify the function signatures and that
	// they don't panic if not executed with privileges
	tests := []struct {
		name     string
		function func() error
	}{
		{"EnableGPUdSystemdUnit", EnableGPUdSystemdUnit},
		{"DisableGPUdSystemdUnit", DisableGPUdSystemdUnit},
		{"RestartGPUdSystemdUnit", RestartGPUdSystemdUnit},
		{"StopSystemdUnit", StopSystemdUnit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if os.Geteuid() != 0 {
				err := test.function()
				if err == nil {
					t.Logf("Note: Expected error for %s when not running as root", test.name)
				} else if !errors.Is(err, errors.ErrUnsupported) {
					// Just verify we get a proper error
					t.Logf("Got expected error: %v", err)
				}
			} else {
				t.Skip("Skipping privileged test")
			}
		})
	}
}
