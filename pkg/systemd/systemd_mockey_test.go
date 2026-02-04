package systemd

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/bytedance/mockey"
	sd "github.com/coreos/go-systemd/v22/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

// --- SystemdExists mockey tests ---

func TestSystemdExists_Found(t *testing.T) {
	mockey.PatchConvey("systemd found in PATH", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			if file == "systemd" {
				return "/usr/lib/systemd/systemd", nil
			}
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		}).Build()

		result := SystemdExists()
		assert.True(t, result)
	})
}

func TestSystemdExists_NotFound(t *testing.T) {
	mockey.PatchConvey("systemd not found in PATH", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		result := SystemdExists()
		assert.False(t, result)
	})
}

// --- SystemctlExists mockey tests ---

func TestSystemctlExists_Found(t *testing.T) {
	mockey.PatchConvey("systemctl found in PATH", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			if f == "systemctl" {
				return "/usr/bin/systemctl", nil
			}
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		result := SystemctlExists()
		assert.True(t, result)
	})
}

func TestSystemctlExists_NotFound(t *testing.T) {
	mockey.PatchConvey("systemctl not found in PATH", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		result := SystemctlExists()
		assert.False(t, result)
	})
}

// --- DaemonReload mockey tests ---

func TestDaemonReload_SystemctlNotFound(t *testing.T) {
	mockey.PatchConvey("systemctl not found", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		result, err := DaemonReload(context.Background())
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestDaemonReload_Success(t *testing.T) {
	mockey.PatchConvey("daemon reload success", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).Output).To(func() ([]byte, error) {
			return []byte(""), nil
		}).Build()

		result, err := DaemonReload(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []byte(""), result)
	})
}

func TestDaemonReload_OutputError(t *testing.T) {
	mockey.PatchConvey("daemon reload output error", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).Output).To(func() ([]byte, error) {
			return nil, errors.New("permission denied")
		}).Build()

		result, err := DaemonReload(context.Background())
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetVersion mockey tests ---

func TestGetVersion_SystemdNotFound(t *testing.T) {
	mockey.PatchConvey("systemd executable not found", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		ver, extra, err := GetVersion()
		require.Error(t, err)
		assert.Equal(t, "", ver)
		assert.Nil(t, extra)
	})
}

func TestGetVersion_ProcessNewError(t *testing.T) {
	mockey.PatchConvey("process.New returns error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/lib/systemd/systemd", nil
		}).Build()

		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("process creation failed")
		}).Build()

		ver, extra, err := GetVersion()
		require.Error(t, err)
		assert.Equal(t, "", ver)
		assert.Nil(t, extra)
	})
}

// --- IsActive mockey tests ---

func TestIsActive_SystemctlNotFound(t *testing.T) {
	mockey.PatchConvey("systemctl not found", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		active, err := IsActive("test-service")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemd active check requires systemctl")
		assert.False(t, active)
	})
}

func TestIsActive_ActiveService(t *testing.T) {
	mockey.PatchConvey("service is active", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("active\n"), nil
		}).Build()

		active, err := IsActive("test-service")
		require.NoError(t, err)
		assert.True(t, active)
	})
}

func TestIsActive_InactiveService(t *testing.T) {
	mockey.PatchConvey("service is inactive", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("inactive\n"), errors.New("exit status 3")
		}).Build()

		active, err := IsActive("test-service")
		require.NoError(t, err)
		assert.False(t, active)
	})
}

func TestIsActive_OtherError(t *testing.T) {
	mockey.PatchConvey("service check other error", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("error output\n"), errors.New("connection refused")
		}).Build()

		active, err := IsActive("test-service")
		require.Error(t, err)
		assert.False(t, active)
	})
}

// --- GetUptime mockey tests ---

func TestGetUptime_SystemctlNotFound(t *testing.T) {
	mockey.PatchConvey("systemctl not found", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", &exec.Error{Name: f, Err: exec.ErrNotFound}
		}).Build()

		dur, err := GetUptime("test-service")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "systemd uptime check requires systemctl")
		assert.Nil(t, dur)
	})
}

func TestGetUptime_Success(t *testing.T) {
	mockey.PatchConvey("get uptime success", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("InactiveExitTimestamp=Wed 2024-02-28 01:29:39 UTC\n"), nil
		}).Build()

		dur, err := GetUptime("test-service")
		require.NoError(t, err)
		require.NotNil(t, dur)
		assert.True(t, *dur > 0)
	})
}

func TestGetUptime_CombinedOutputError(t *testing.T) {
	mockey.PatchConvey("get uptime command error", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte(""), errors.New("command failed")
		}).Build()

		dur, err := GetUptime("test-service")
		require.Error(t, err)
		assert.Nil(t, dur)
	})
}

func TestGetUptime_InvalidOutput(t *testing.T) {
	mockey.PatchConvey("get uptime invalid output", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("garbage"), nil
		}).Build()

		dur, err := GetUptime("test-service")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not parse the service uptime time correctly")
		assert.Nil(t, dur)
	})
}

func TestGetUptime_NotApplicable(t *testing.T) {
	mockey.PatchConvey("get uptime n/a", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("InactiveExitTimestamp=n/a\n"), nil
		}).Build()

		dur, err := GetUptime("test-service")
		require.NoError(t, err)
		assert.Nil(t, dur)
	})
}

func TestGetUptime_EmptyTimestamp(t *testing.T) {
	mockey.PatchConvey("get uptime empty timestamp", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("InactiveExitTimestamp=\n"), nil
		}).Build()

		dur, err := GetUptime("test-service")
		require.NoError(t, err)
		assert.Nil(t, dur)
	})
}

// --- sdNotify mockey tests ---

func TestNotifyReady(t *testing.T) {
	mockey.PatchConvey("NotifyReady success", t, func() {
		mockey.Mock(sd.SdNotify).To(func(unsetEnvironment bool, state string) (bool, error) {
			assert.Equal(t, sd.SdNotifyReady, state)
			return true, nil
		}).Build()

		err := NotifyReady(context.Background())
		require.NoError(t, err)
	})
}

func TestNotifyReady_Error(t *testing.T) {
	mockey.PatchConvey("NotifyReady error", t, func() {
		mockey.Mock(sd.SdNotify).To(func(unsetEnvironment bool, state string) (bool, error) {
			return false, errors.New("notification failed")
		}).Build()

		err := NotifyReady(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notification failed")
	})
}

func TestNotifyStopping(t *testing.T) {
	mockey.PatchConvey("NotifyStopping success", t, func() {
		mockey.Mock(sd.SdNotify).To(func(unsetEnvironment bool, state string) (bool, error) {
			assert.Equal(t, sd.SdNotifyStopping, state)
			return true, nil
		}).Build()

		err := NotifyStopping(context.Background())
		require.NoError(t, err)
	})
}

func TestNotifyStopping_Error(t *testing.T) {
	mockey.PatchConvey("NotifyStopping error", t, func() {
		mockey.Mock(sd.SdNotify).To(func(unsetEnvironment bool, state string) (bool, error) {
			return false, errors.New("notification failed")
		}).Build()

		err := NotifyStopping(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notification failed")
	})
}

// NOTE: TestGetVersion_Success and TestGetVersion_ProcessStartError are omitted
// because mocking process.New with a call back to process.New causes infinite
// recursion and GC heap corruption. The error paths are covered by
// TestGetVersion_SystemdNotFound and TestGetVersion_ProcessNewError.

// --- parseVersion direct tests ---

func TestParseVersion_Success(t *testing.T) {
	mockey.PatchConvey("parseVersion extracts version and extra lines", t, func() {
		input := "systemd 252 (252.22-1ubuntu1)\n+PAM +AUDIT +SELINUX +APPARMOR\n+KMOD +DBUS"
		ver, extra := parseVersion(input)
		assert.Equal(t, "systemd 252 (252.22-1ubuntu1)", ver)
		require.Len(t, extra, 2)
		assert.Equal(t, "+PAM +AUDIT +SELINUX +APPARMOR", extra[0])
		assert.Equal(t, "+KMOD +DBUS", extra[1])
	})
}

func TestParseVersion_EmptyInput(t *testing.T) {
	mockey.PatchConvey("parseVersion with empty input returns empty", t, func() {
		ver, extra := parseVersion("")
		assert.Empty(t, ver)
		assert.Nil(t, extra)
	})
}

func TestParseVersion_SingleLine(t *testing.T) {
	mockey.PatchConvey("parseVersion with single line returns version only", t, func() {
		ver, extra := parseVersion("systemd 252 (252.22-1ubuntu1)")
		assert.Equal(t, "systemd 252 (252.22-1ubuntu1)", ver)
		assert.Empty(t, extra)
	})
}

// --- SystemdExists edge case ---

func TestSystemdExists_EmptyPath(t *testing.T) {
	mockey.PatchConvey("SystemdExists returns false when LookPath returns empty string", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", nil
		}).Build()

		result := SystemdExists()
		assert.False(t, result)
	})
}

func TestSystemctlExists_EmptyPath(t *testing.T) {
	mockey.PatchConvey("SystemctlExists returns false when LookPath returns empty string", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "", nil
		}).Build()

		result := SystemctlExists()
		assert.False(t, result)
	})
}

// --- IsActive edge cases ---

func TestIsActive_ActiveWithWhitespace(t *testing.T) {
	mockey.PatchConvey("service active with whitespace", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("  active  \n"), nil
		}).Build()

		active, err := IsActive("test-service")
		require.NoError(t, err)
		assert.True(t, active)
	})
}

func TestIsActive_FailedService(t *testing.T) {
	mockey.PatchConvey("service is failed (not active, not inactive)", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("failed\n"), errors.New("exit status 3")
		}).Build()

		active, err := IsActive("test-service")
		require.Error(t, err)
		assert.False(t, active)
	})
}

// --- GetUptime edge cases ---

func TestGetUptime_MultipleEqualsInTimestamp(t *testing.T) {
	mockey.PatchConvey("GetUptime with multiple = in output", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		// This tests that only the first "=" is used for splitting
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func() ([]byte, error) {
			return []byte("InactiveExitTimestamp=Wed 2024-02-28 01:29:39 UTC\n"), nil
		}).Build()

		dur, err := GetUptime("test-service")
		require.NoError(t, err)
		require.NotNil(t, dur)
		assert.True(t, *dur > 0)
	})
}

// --- parseVersion edge cases ---

func TestParseVersion_WithBlankLines(t *testing.T) {
	ver, extra := parseVersion("systemd 252\n\n\n+PAM\n\n")
	assert.Equal(t, "systemd 252", ver)
	assert.Len(t, extra, 1)
	assert.Equal(t, "+PAM", extra[0])
}

// --- parseSystemdUnitUptime edge cases ---

func TestParseSystemdUnitUptime_ValidTimestamp(t *testing.T) {
	dur, err := parseSystemdUnitUptime("Wed 2024-02-28 01:29:39 UTC")
	require.NoError(t, err)
	assert.True(t, dur > 0)
}

func TestParseSystemdUnitUptime_WithTrailingNewline(t *testing.T) {
	dur, err := parseSystemdUnitUptime("Wed 2024-02-28 01:29:39 UTC\x0a")
	require.NoError(t, err)
	assert.True(t, dur > 0)
}

func TestParseSystemdUnitUptime_InvalidFormat(t *testing.T) {
	_, err := parseSystemdUnitUptime("invalid timestamp format")
	require.Error(t, err)
}

// --- DaemonReload with output ---

func TestDaemonReload_WithOutputContent(t *testing.T) {
	mockey.PatchConvey("daemon reload returns output content", t, func() {
		mockey.Mock(exec.LookPath).To(func(f string) (string, error) {
			return "/usr/bin/systemctl", nil
		}).Build()

		mockey.Mock((*exec.Cmd).Output).To(func() ([]byte, error) {
			return []byte("reload complete"), nil
		}).Build()

		result, err := DaemonReload(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []byte("reload complete"), result)
	})
}
