package update

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
)

// TestDetectUbuntuVersion_Ubuntu2204 tests Ubuntu 22.04 detection.
func TestDetectUbuntuVersion_Ubuntu2204(t *testing.T) {
	mockey.PatchConvey("detect ubuntu 22.04", t, func() {
		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				// First call: lsb_release -i -s
				return []byte("Ubuntu\n"), nil
			}
			// Second call: lsb_release -r -s
			return []byte("22.04\n"), nil
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "ubuntu22.04", result)
	})
}

// TestDetectUbuntuVersion_Ubuntu2404 tests Ubuntu 24.04 detection.
func TestDetectUbuntuVersion_Ubuntu2404(t *testing.T) {
	mockey.PatchConvey("detect ubuntu 24.04", t, func() {
		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				return []byte("Ubuntu\n"), nil
			}
			return []byte("24.04\n"), nil
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "ubuntu24.04", result)
	})
}

// TestDetectUbuntuVersion_Ubuntu2604 tests Ubuntu 26.04 detection.
func TestDetectUbuntuVersion_Ubuntu2604(t *testing.T) {
	mockey.PatchConvey("detect ubuntu 26.04", t, func() {
		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				return []byte("Ubuntu\n"), nil
			}
			return []byte("26.04\n"), nil
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "ubuntu26.04", result)
	})
}

// TestDetectUbuntuVersion_NotUbuntu tests when OS is not Ubuntu.
func TestDetectUbuntuVersion_NotUbuntu(t *testing.T) {
	mockey.PatchConvey("not ubuntu", t, func() {
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Debian\n"), nil
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "", result)
	})
}

// TestDetectUbuntuVersion_UnsupportedVersion tests when Ubuntu version is not supported.
func TestDetectUbuntuVersion_UnsupportedVersion(t *testing.T) {
	mockey.PatchConvey("unsupported ubuntu version", t, func() {
		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				return []byte("Ubuntu\n"), nil
			}
			return []byte("20.04\n"), nil
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "", result)
	})
}

// useOSRelease points detectUbuntuVersion at an os-release fixture with the
// given content, or at a missing file when content is empty.
func useOSRelease(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if content != "" {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	orig := osReleasePath
	osReleasePath = path
	t.Cleanup(func() { osReleasePath = orig })
}

// TestDetectUbuntuVersion_LsbReleaseError tests when lsb_release command fails
// and no os-release file exists.
func TestDetectUbuntuVersion_LsbReleaseError(t *testing.T) {
	useOSRelease(t, "")
	mockey.PatchConvey("lsb_release error", t, func() {
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errors.New("command not found")
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "", result)
	})
}

// TestDetectUbuntuVersion_VersionCommandError tests when getting version fails
// and no os-release file exists.
func TestDetectUbuntuVersion_VersionCommandError(t *testing.T) {
	useOSRelease(t, "")
	mockey.PatchConvey("version command error", t, func() {
		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				return []byte("Ubuntu\n"), nil
			}
			return nil, errors.New("failed to get version")
		}).Build()

		result := detectUbuntuVersion()
		assert.Equal(t, "", result)
	})
}

// TestDetectUbuntuVersion_OSReleaseFallback tests the os-release fallback when
// lsb_release cannot run, as in the GPUd container image.
func TestDetectUbuntuVersion_OSReleaseFallback(t *testing.T) {
	for _, tt := range []struct {
		name, osRelease, want string
	}{
		{name: "ubuntu 22.04", osRelease: "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"22.04\"\n", want: "ubuntu22.04"},
		{name: "ubuntu 24.04", osRelease: "ID=ubuntu\nVERSION_ID=\"24.04\"\n", want: "ubuntu24.04"},
		{name: "ubuntu 26.04", osRelease: "ID=ubuntu\nVERSION_ID=26.04\n", want: "ubuntu26.04"},
		{name: "single quotes and comments", osRelease: "# generated\n\nID='ubuntu'\nVERSION_ID='22.04'\n", want: "ubuntu22.04"},
		{name: "unsupported ubuntu", osRelease: "ID=ubuntu\nVERSION_ID=\"20.04\"\n", want: ""},
		{name: "not ubuntu", osRelease: "ID=rhel\nID_LIKE=\"fedora\"\nVERSION_ID=\"9.4\"\n", want: ""},
		{name: "missing file", want: ""},
		// A file that cannot be read to the end is not trusted, even if the
		// fields came before the bad line.
		{name: "unreadable line", osRelease: "ID=ubuntu\nVERSION_ID=22.04\nPADDING=" + strings.Repeat("a", bufio.MaxScanTokenSize) + "\n", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			useOSRelease(t, tt.osRelease)
			mockey.PatchConvey(tt.name, t, func() {
				mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
					return nil, exec.ErrNotFound
				}).Build()

				assert.Equal(t, tt.want, detectUbuntuVersion())
			})
		})
	}
}

// TestDetectUbuntuVersion_LsbReleasePreferred tests that a working
// lsb_release wins over os-release.
func TestDetectUbuntuVersion_LsbReleasePreferred(t *testing.T) {
	useOSRelease(t, "ID=ubuntu\nVERSION_ID=\"24.04\"\n")
	mockey.PatchConvey("lsb_release preferred", t, func() {
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Debian\n"), nil
		}).Build()

		assert.Equal(t, "", detectUbuntuVersion())
	})
}

// TestEnableGPUdSystemdUnit_SystemctlNotExists tests when systemctl doesn't exist.
func TestEnableGPUdSystemdUnit_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := EnableGPUdSystemdUnit()
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrUnsupported)
	})
}

// TestEnableGPUdSystemdUnit_Success tests successful enable.
func TestEnableGPUdSystemdUnit_Success(t *testing.T) {
	mockey.PatchConvey("enable success", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		err := EnableGPUdSystemdUnit()
		require.NoError(t, err)
	})
}

// TestEnableGPUdSystemdUnit_Error tests when enable fails.
func TestEnableGPUdSystemdUnit_Error(t *testing.T) {
	mockey.PatchConvey("enable error", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Failed to enable unit"), errors.New("exit status 1")
		}).Build()

		err := EnableGPUdSystemdUnit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemctl enable failed")
	})
}

// TestDisableGPUdSystemdUnit_SystemctlNotExists tests when systemctl doesn't exist.
func TestDisableGPUdSystemdUnit_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := DisableGPUdSystemdUnit()
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrUnsupported)
	})
}

// TestDisableGPUdSystemdUnit_Success tests successful disable.
func TestDisableGPUdSystemdUnit_Success(t *testing.T) {
	mockey.PatchConvey("disable success", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		err := DisableGPUdSystemdUnit()
		require.NoError(t, err)
	})
}

// TestDisableGPUdSystemdUnit_Error tests when disable fails.
func TestDisableGPUdSystemdUnit_Error(t *testing.T) {
	mockey.PatchConvey("disable error", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Failed to disable unit"), errors.New("exit status 1")
		}).Build()

		err := DisableGPUdSystemdUnit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemctl disable failed")
	})
}

// TestRestartGPUdSystemdUnit_SystemctlNotExists tests when systemctl doesn't exist.
func TestRestartGPUdSystemdUnit_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := RestartGPUdSystemdUnit()
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrUnsupported)
	})
}

// TestRestartGPUdSystemdUnit_Success tests successful restart.
func TestRestartGPUdSystemdUnit_Success(t *testing.T) {
	mockey.PatchConvey("restart success", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		err := RestartGPUdSystemdUnit()
		require.NoError(t, err)
	})
}

// TestRestartGPUdSystemdUnit_DaemonReloadError tests when daemon-reload fails.
func TestRestartGPUdSystemdUnit_DaemonReloadError(t *testing.T) {
	mockey.PatchConvey("daemon-reload error", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Failed to reload daemon"), errors.New("exit status 1")
		}).Build()

		err := RestartGPUdSystemdUnit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemctl daemon-reload failed")
	})
}

// TestRestartGPUdSystemdUnit_RestartError tests when restart fails.
func TestRestartGPUdSystemdUnit_RestartError(t *testing.T) {
	mockey.PatchConvey("restart error", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		cmdCallCount := 0
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			cmdCallCount++
			if cmdCallCount == 1 {
				// daemon-reload succeeds
				return []byte(""), nil
			}
			// restart fails
			return []byte("Failed to restart gpud.service"), errors.New("exit status 1")
		}).Build()

		err := RestartGPUdSystemdUnit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemctl restart failed")
	})
}

// TestStopSystemdUnit_SystemctlNotExists tests when systemctl doesn't exist.
func TestStopSystemdUnit_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := StopSystemdUnit()
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrUnsupported)
	})
}

// TestStopSystemdUnit_Success tests successful stop.
func TestStopSystemdUnit_Success(t *testing.T) {
	mockey.PatchConvey("stop success", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		err := StopSystemdUnit()
		require.NoError(t, err)
	})
}

// TestStopSystemdUnit_Error tests when stop fails.
func TestStopSystemdUnit_Error(t *testing.T) {
	mockey.PatchConvey("stop error", t, func() {
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Failed to stop gpud.service"), errors.New("exit status 1")
		}).Build()

		err := StopSystemdUnit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemctl stop failed")
	})
}
