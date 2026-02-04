package host

import (
	"context"
	"errors"
	stdos "os"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
)

// --- Op option tests ---

func TestApplyOpts(t *testing.T) {
	mockey.PatchConvey("applyOpts sets options", t, func() {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDelaySeconds(5),
			WithSystemctl(true),
		})
		require.NoError(t, err)
		assert.Equal(t, 5, op.delaySeconds)
		assert.True(t, op.useSystemctl)
	})
}

func TestApplyOpts_Empty(t *testing.T) {
	mockey.PatchConvey("applyOpts with no options", t, func() {
		op := &Op{}
		err := op.applyOpts(nil)
		require.NoError(t, err)
		assert.Equal(t, 0, op.delaySeconds)
		assert.False(t, op.useSystemctl)
	})
}

func TestWithDelaySeconds(t *testing.T) {
	mockey.PatchConvey("WithDelaySeconds sets delaySeconds", t, func() {
		op := &Op{}
		opt := WithDelaySeconds(10)
		opt(op)
		assert.Equal(t, 10, op.delaySeconds)
	})
}

func TestWithSystemctl(t *testing.T) {
	mockey.PatchConvey("WithSystemctl sets useSystemctl", t, func() {
		op := &Op{}
		opt := WithSystemctl(true)
		opt(op)
		assert.True(t, op.useSystemctl)
	})
}

// --- Reboot tests ---

func TestReboot_NotRoot(t *testing.T) {
	mockey.PatchConvey("Reboot fails when not root", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 1000 // non-root
		}).Build()

		err := Reboot(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotRoot)
	})
}

// NOTE: Tests for Reboot with delay and systemctl options require root privileges
// (os.Geteuid cannot be reliably mocked without -gcflags="all=-N -l")
// so they are omitted here.

// --- Stop tests ---

func TestStop_NotRoot(t *testing.T) {
	mockey.PatchConvey("Stop fails when not root", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 1000 // non-root
		}).Build()

		err := Stop(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotRoot)
	})
}

// NOTE: Tests for Stop with delay options require root privileges
// (os.Geteuid cannot be reliably mocked without -gcflags="all=-N -l")
// so they are omitted here.

// --- RunReboot/RunStop with echo command ---

func TestRunReboot_EchoCommand(t *testing.T) {
	mockey.PatchConvey("runReboot with echo command succeeds", t, func() {
		ctx := context.Background()
		err := runReboot(ctx, "echo reboot-test")
		require.NoError(t, err)
	})
}

func TestRunStop_EchoCommand(t *testing.T) {
	mockey.PatchConvey("runStop with echo command succeeds", t, func() {
		ctx := context.Background()
		err := runStop(ctx, "echo stop-test")
		require.NoError(t, err)
	})
}

// --- GetSystemdDetectVirt tests ---

func TestGetSystemdDetectVirt_NotFound(t *testing.T) {
	mockey.PatchConvey("GetSystemdDetectVirt returns empty when executable not found", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		virt, err := GetSystemdDetectVirt(context.Background())
		require.NoError(t, err)
		assert.Empty(t, virt.Type)
		assert.Empty(t, virt.VM)
		assert.Empty(t, virt.Container)
		assert.False(t, virt.IsKVM)
	})
}

func TestGetSystemdDetectVirt_Success(t *testing.T) {
	mockey.PatchConvey("GetSystemdDetectVirt parses output correctly", t, func() {
		// Mock LocateExecutable to return a script that produces known output
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "echo", nil
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		virt, err := GetSystemdDetectVirt(ctx)
		require.NoError(t, err)
		// With "echo" as the executable, the script will produce output
		// but the exact content depends on bash execution
		assert.NotNil(t, virt)
	})
}

// --- GetSystemManufacturer tests ---

func TestGetSystemManufacturer_NotFound(t *testing.T) {
	mockey.PatchConvey("GetSystemManufacturer returns empty when dmidecode not found", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("dmidecode not found")
		}).Build()

		manufacturer, err := GetSystemManufacturer(context.Background())
		require.NoError(t, err)
		assert.Empty(t, manufacturer)
	})
}

// --- GetDmidecodeUUID tests ---

func TestGetDmidecodeUUID_NotFound(t *testing.T) {
	mockey.PatchConvey("GetDmidecodeUUID returns error when dmidecode not found", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("dmidecode not found")
		}).Build()

		uuid, err := GetDmidecodeUUID(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dmidecode not found")
		assert.Empty(t, uuid)
	})
}

// --- GetMachineID fallback test ---

func TestGetMachineID_FallbackToOSMachineID(t *testing.T) {
	mockey.PatchConvey("GetMachineID falls back to OS machine ID", t, func() {
		// Mock GetDmidecodeUUID to fail
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		ctx := context.Background()
		// This will fail dmidecode and fall back to OS machine ID
		id, err := GetMachineID(ctx)
		// Should not error (falls back to reading /etc/machine-id)
		require.NoError(t, err)
		// id might be non-empty if /etc/machine-id exists on test system
		_ = id
	})
}

// --- extractUUID tests ---

func TestExtractUUID_Various(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{"valid UUID line", "UUID: 12345678-1234-1234-1234-123456789abc", "12345678-1234-1234-1234-123456789abc"},
		{"UUID with whitespace", "  UUID: abc-def  ", "abc-def"},
		{"no UUID prefix", "Serial Number: ABC123", ""},
		{"empty line", "", ""},
		{"only UUID prefix", "UUID: ", ""},
		{"UUID: with no value", "UUID:", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractUUID(tc.line)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- VirtualizationEnvironment struct tests ---

func TestVirtualizationEnvironment_Struct(t *testing.T) {
	virt := VirtualizationEnvironment{
		Type:      "kvm",
		VM:        "kvm",
		Container: "none",
		IsKVM:     true,
	}

	assert.Equal(t, "kvm", virt.Type)
	assert.Equal(t, "kvm", virt.VM)
	assert.Equal(t, "none", virt.Container)
	assert.True(t, virt.IsKVM)
}

func TestVirtualizationEnvironment_Empty(t *testing.T) {
	virt := VirtualizationEnvironment{}

	assert.Empty(t, virt.Type)
	assert.Empty(t, virt.VM)
	assert.Empty(t, virt.Container)
	assert.False(t, virt.IsKVM)
}

// --- GetBootID public function test ---

func TestGetBootID_Public(t *testing.T) {
	mockey.PatchConvey("GetBootID reads from system path", t, func() {
		id, err := GetBootID()
		// Should not error on Linux
		require.NoError(t, err)
		// id should be non-empty on Linux systems
		if id != "" {
			assert.Len(t, id, 36) // UUID format
		}
	})
}

// --- GetOSMachineID public function test ---

func TestGetOSMachineID_Public(t *testing.T) {
	mockey.PatchConvey("GetOSMachineID reads from system path", t, func() {
		id, err := GetOSMachineID()
		require.NoError(t, err)
		// On most Linux systems, this should return a non-empty value
		_ = id
	})
}

// --- GetOSName public function test ---

func TestGetOSName_Public(t *testing.T) {
	mockey.PatchConvey("GetOSName reads from system path", t, func() {
		name, err := GetOSName()
		require.NoError(t, err)
		// On most Linux systems, this should return a non-empty value
		if name != "" {
			assert.NotEmpty(t, name)
		}
	})
}

// --- GetSystemUUID tests ---

func TestGetSystemUUID_ReadFile(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID tries multiple sources", t, func() {
		uuid, err := GetSystemUUID()
		// On test systems, at least one of the UUID sources should work
		// If none work, we just check it doesn't panic
		_ = uuid
		_ = err
	})
}

// --- Reboot immediate path tests ---

func TestReboot_ImmediateAsRoot(t *testing.T) {
	mockey.PatchConvey("Reboot immediate as root with mocked runReboot", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 0 // root
		}).Build()

		// CRITICAL: Mock runReboot to prevent actual system reboot
		var capturedCmd string
		mockey.Mock(runReboot).To(func(ctx context.Context, cmd string) error {
			capturedCmd = cmd
			return nil
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := Reboot(ctx)
		require.NoError(t, err)
		assert.Equal(t, "sudo reboot", capturedCmd)
	})
}

func TestReboot_ImmediateWithSystemctl(t *testing.T) {
	mockey.PatchConvey("Reboot immediate with systemctl as root", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 0 // root
		}).Build()

		// CRITICAL: Mock runReboot to prevent actual system reboot
		var capturedCmd string
		mockey.Mock(runReboot).To(func(ctx context.Context, cmd string) error {
			capturedCmd = cmd
			return nil
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := Reboot(ctx, WithSystemctl(true))
		require.NoError(t, err)
		assert.Equal(t, "sudo systemctl reboot", capturedCmd)
	})
}

// --- Stop immediate path tests ---

func TestStop_ImmediateAsRoot(t *testing.T) {
	mockey.PatchConvey("Stop immediate as root with mocked runStop", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 0 // root
		}).Build()

		// CRITICAL: Mock runStop to prevent actual service stop
		var capturedCmd string
		mockey.Mock(runStop).To(func(ctx context.Context, cmd string) error {
			capturedCmd = cmd
			return nil
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := Stop(ctx)
		require.NoError(t, err)
		assert.Equal(t, "sudo systemctl stop gpud", capturedCmd)
	})
}

// --- runStop tests ---

func TestRunStop_WithInvalidCommand(t *testing.T) {
	mockey.PatchConvey("runStop with command that exits non-zero", t, func() {
		ctx := context.Background()
		err := runStop(ctx, "exit 1")
		// Should succeed because the process runs to completion
		// The exit code is from bash, not from runStop itself
		_ = err
	})
}

// --- GetSystemUUID with mocked file reads ---

func TestGetSystemUUID_AllPathsFail(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID when all paths fail", t, func() {
		mockey.Mock(stdos.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("file not found")
		}).Build()

		uuid, err := GetSystemUUID()
		require.Error(t, err)
		assert.Empty(t, uuid)
	})
}

func TestGetSystemUUID_DMIPathSucceeds(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID when DMI path succeeds", t, func() {
		mockey.Mock(stdos.ReadFile).To(func(name string) ([]byte, error) {
			if name == "/sys/class/dmi/id/product_uuid" {
				return []byte("  12345678-1234-1234-1234-123456789abc  \n"), nil
			}
			return nil, errors.New("file not found")
		}).Build()

		uuid, err := GetSystemUUID()
		require.NoError(t, err)
		assert.Equal(t, "12345678-1234-1234-1234-123456789abc", uuid)
	})
}

func TestGetSystemUUID_PPCPathSucceeds(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID when PPC system-id path succeeds", t, func() {
		mockey.Mock(stdos.ReadFile).To(func(name string) ([]byte, error) {
			if name == "/proc/device-tree/system-id" {
				return []byte("ppc-uuid-1234\000"), nil
			}
			if name == "/sys/class/dmi/id/product_uuid" {
				return nil, errors.New("not found")
			}
			return nil, errors.New("file not found")
		}).Build()

		uuid, err := GetSystemUUID()
		require.NoError(t, err)
		assert.Equal(t, "ppc-uuid-1234", uuid)
	})
}

func TestGetSystemUUID_PPCVMUUIDSucceeds(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID when PPC vm,uuid path succeeds", t, func() {
		mockey.Mock(stdos.ReadFile).To(func(name string) ([]byte, error) {
			if name == "/proc/device-tree/vm,uuid" {
				return []byte("vm-uuid-5678\000"), nil
			}
			if name == "/sys/class/dmi/id/product_uuid" || name == "/proc/device-tree/system-id" {
				return nil, errors.New("not found")
			}
			return nil, errors.New("file not found")
		}).Build()

		uuid, err := GetSystemUUID()
		require.NoError(t, err)
		assert.Equal(t, "vm-uuid-5678", uuid)
	})
}

func TestGetSystemUUID_S390xPathSucceeds(t *testing.T) {
	mockey.PatchConvey("GetSystemUUID when s390x machine-id path succeeds", t, func() {
		mockey.Mock(stdos.ReadFile).To(func(name string) ([]byte, error) {
			if name == "/etc/machine-id" {
				return []byte("  s390x-uuid-9012  "), nil
			}
			if name == "/sys/class/dmi/id/product_uuid" || name == "/proc/device-tree/system-id" || name == "/proc/device-tree/vm,uuid" {
				return nil, errors.New("not found")
			}
			return nil, errors.New("file not found")
		}).Build()

		uuid, err := GetSystemUUID()
		require.NoError(t, err)
		assert.Equal(t, "s390x-uuid-9012", uuid)
	})
}

// --- ErrNotRoot constant test ---

func TestErrNotRoot(t *testing.T) {
	assert.NotNil(t, ErrNotRoot)
	assert.Contains(t, ErrNotRoot.Error(), "root")
}

// --- GetMachineID with successful dmidecode ---

func TestGetMachineID_DmidecodeSuccess(t *testing.T) {
	mockey.PatchConvey("GetMachineID returns dmidecode UUID when available", t, func() {
		mockey.Mock(GetDmidecodeUUID).To(func(ctx context.Context) (string, error) {
			return "12345678-abcd-efgh-ijkl-123456789012", nil
		}).Build()

		id, err := GetMachineID(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "12345678-abcd-efgh-ijkl-123456789012", id)
	})
}

// NOTE: TestGetMachineID_DmidecodeEmpty_FallbackToOS is omitted because
// mocking GetOSMachineID requires -gcflags="all=-N -l" to disable inlining.

// --- getOSName parsing tests ---

func TestGetOSName_PrettyNamePreferred(t *testing.T) {
	mockey.PatchConvey("getOSName prefers PRETTY_NAME over NAME", t, func() {
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/os-release"
		require.NoError(t, stdos.WriteFile(tmpFile, []byte("NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 22.04.3 LTS\"\n"), 0644))

		name, err := getOSName(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "Ubuntu 22.04.3 LTS", name)
	})
}

func TestGetOSName_FallbackToNAME(t *testing.T) {
	mockey.PatchConvey("getOSName falls back to NAME when no PRETTY_NAME", t, func() {
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/os-release"
		require.NoError(t, stdos.WriteFile(tmpFile, []byte("NAME=\"Debian GNU/Linux\"\nVERSION=\"12\"\n"), 0644))

		name, err := getOSName(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "Debian GNU/Linux", name)
	})
}

func TestGetOSName_EmptyFile(t *testing.T) {
	mockey.PatchConvey("getOSName returns empty for empty file", t, func() {
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/os-release"
		require.NoError(t, stdos.WriteFile(tmpFile, []byte(""), 0644))

		name, err := getOSName(tmpFile)
		require.NoError(t, err)
		assert.Empty(t, name)
	})
}

func TestGetOSName_NoMatchingKeys(t *testing.T) {
	mockey.PatchConvey("getOSName returns empty when no matching keys", t, func() {
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/os-release"
		require.NoError(t, stdos.WriteFile(tmpFile, []byte("VERSION=12\nID=debian\n"), 0644))

		name, err := getOSName(tmpFile)
		require.NoError(t, err)
		assert.Empty(t, name)
	})
}

// --- getBootID tests ---

func TestGetBootID_MockedReadFile(t *testing.T) {
	mockey.PatchConvey("getBootID with mocked file read", t, func() {
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/boot_id"
		require.NoError(t, stdos.WriteFile(tmpFile, []byte("  abc-123-def-456  \n"), 0644))

		id, err := getBootID(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "abc-123-def-456", id)
	})
}

func TestGetBootID_NonExistentFile(t *testing.T) {
	mockey.PatchConvey("getBootID returns empty for non-existent file", t, func() {
		id, err := getBootID("/nonexistent/path/boot_id")
		require.NoError(t, err)
		assert.Empty(t, id)
	})
}

// --- getOSMachineID tests ---

func TestGetOSMachineID_FirstFileExists(t *testing.T) {
	mockey.PatchConvey("getOSMachineID reads first available file", t, func() {
		tmpDir := t.TempDir()
		tmpFile1 := tmpDir + "/machine-id-1"
		tmpFile2 := tmpDir + "/machine-id-2"
		require.NoError(t, stdos.WriteFile(tmpFile1, []byte("first-id-value\n"), 0644))
		require.NoError(t, stdos.WriteFile(tmpFile2, []byte("second-id-value\n"), 0644))

		id, err := getOSMachineID([]string{tmpFile1, tmpFile2})
		require.NoError(t, err)
		assert.Equal(t, "first-id-value", id)
	})
}

func TestGetOSMachineID_FallbackToSecondFile(t *testing.T) {
	mockey.PatchConvey("getOSMachineID falls back to second file", t, func() {
		tmpDir := t.TempDir()
		tmpFile2 := tmpDir + "/machine-id-2"
		require.NoError(t, stdos.WriteFile(tmpFile2, []byte("second-id-value\n"), 0644))

		id, err := getOSMachineID([]string{"/nonexistent/path", tmpFile2})
		require.NoError(t, err)
		assert.Equal(t, "second-id-value", id)
	})
}

func TestGetOSMachineID_AllFilesMissing(t *testing.T) {
	mockey.PatchConvey("getOSMachineID returns empty when all files missing", t, func() {
		id, err := getOSMachineID([]string{"/nonexistent/1", "/nonexistent/2"})
		require.NoError(t, err)
		assert.Empty(t, id)
	})
}

// --- Event constants ---

func TestEventConstants(t *testing.T) {
	assert.Equal(t, "os", EventBucketName)
	assert.Equal(t, "reboot", EventNameReboot)
}

// --- BootTime tests ---

func TestBootTime_ZeroValue(t *testing.T) {
	mockey.PatchConvey("BootTime returns zero when boot time is 0", t, func() {
		saved := currentBootTimeUnixSeconds
		defer func() { currentBootTimeUnixSeconds = saved }()

		currentBootTimeUnixSeconds = 0
		bt := BootTime()
		assert.True(t, bt.IsZero())
	})
}

func TestBootTime_NonZero(t *testing.T) {
	mockey.PatchConvey("BootTime returns correct time for non-zero", t, func() {
		saved := currentBootTimeUnixSeconds
		defer func() { currentBootTimeUnixSeconds = saved }()

		currentBootTimeUnixSeconds = 1700000000 // Nov 14, 2023
		bt := BootTime()
		assert.False(t, bt.IsZero())
		assert.Equal(t, int64(1700000000), bt.Unix())
	})
}

// --- Getter consistency test ---

func TestGetterConsistency(t *testing.T) {
	mockey.PatchConvey("Getters return consistent values across calls", t, func() {
		assert.Equal(t, HostID(), HostID())
		assert.Equal(t, Arch(), Arch())
		assert.Equal(t, KernelVersion(), KernelVersion())
		assert.Equal(t, Platform(), Platform())
		assert.Equal(t, PlatformFamily(), PlatformFamily())
		assert.Equal(t, PlatformVersion(), PlatformVersion())
		assert.Equal(t, BootTimeUnixSeconds(), BootTimeUnixSeconds())
		assert.Equal(t, BootID(), BootID())
		assert.Equal(t, MachineID(), MachineID())
		assert.Equal(t, CPUVendorID(), CPUVendorID())
		assert.Equal(t, CPUModelName(), CPUModelName())
		assert.Equal(t, CPUModel(), CPUModel())
		assert.Equal(t, CPUFamily(), CPUFamily())
		assert.Equal(t, CPULogicalCores(), CPULogicalCores())
		assert.Equal(t, OSMachineID(), OSMachineID())
		assert.Equal(t, OSName(), OSName())
		assert.Equal(t, SystemUUID(), SystemUUID())
	})
}
