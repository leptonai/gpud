package host

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestArch tests that Arch() returns a non-empty architecture string.
func TestArch(t *testing.T) {
	arch := Arch()
	assert.NotEmpty(t, arch, "Arch should return a non-empty string")
	t.Logf("Architecture: %s", arch)
}

// TestCPUVendorID tests that CPUVendorID() returns a string (may be empty on some systems).
func TestCPUVendorID(t *testing.T) {
	vendorID := CPUVendorID()
	// May be empty on some systems, but should not panic
	t.Logf("CPU Vendor ID: %s", vendorID)
}

// TestCPUModelName tests that CPUModelName() returns a string.
func TestCPUModelName(t *testing.T) {
	modelName := CPUModelName()
	t.Logf("CPU Model Name: %s", modelName)
}

// TestCPUModel tests that CPUModel() returns a string.
func TestCPUModel(t *testing.T) {
	model := CPUModel()
	t.Logf("CPU Model: %s", model)
}

// TestCPUFamily tests that CPUFamily() returns a string.
func TestCPUFamily(t *testing.T) {
	family := CPUFamily()
	t.Logf("CPU Family: %s", family)
}

// TestCPULogicalCores tests that CPULogicalCores() returns a positive number.
func TestCPULogicalCores(t *testing.T) {
	cores := CPULogicalCores()
	assert.Greater(t, cores, 0, "CPU logical cores should be positive")
	t.Logf("CPU Logical Cores: %d", cores)
}

// TestKernelVersion tests that KernelVersion() returns a non-empty string.
func TestKernelVersion(t *testing.T) {
	version := KernelVersion()
	assert.NotEmpty(t, version, "Kernel version should not be empty")
	t.Logf("Kernel Version: %s", version)
}

// TestPlatform tests that Platform() returns a non-empty string.
func TestPlatform(t *testing.T) {
	platform := Platform()
	assert.NotEmpty(t, platform, "Platform should not be empty")
	t.Logf("Platform: %s", platform)
}

// TestPlatformFamily tests that PlatformFamily() returns a string.
func TestPlatformFamily(t *testing.T) {
	family := PlatformFamily()
	t.Logf("Platform Family: %s", family)
}

// TestPlatformVersion tests that PlatformVersion() returns a string.
func TestPlatformVersion(t *testing.T) {
	version := PlatformVersion()
	t.Logf("Platform Version: %s", version)
}

// TestBootTimeUnixSeconds tests that BootTimeUnixSeconds() returns a reasonable timestamp.
func TestBootTimeUnixSeconds(t *testing.T) {
	bootTime := BootTimeUnixSeconds()
	// Boot time should be in the past (less than current time)
	now := uint64(time.Now().Unix())
	if bootTime > 0 {
		assert.Less(t, bootTime, now, "Boot time should be in the past")
	}
	t.Logf("Boot Time Unix Seconds: %d", bootTime)
}

// TestBootTimeGetter tests the BootTime() getter function.
func TestBootTimeGetter(t *testing.T) {
	bootTime := BootTime()

	// If boot time is zero, it means it couldn't be determined
	if bootTime.IsZero() {
		t.Log("Boot time could not be determined (zero value)")
		return
	}

	// Boot time should be in the past
	assert.True(t, bootTime.Before(time.Now()), "Boot time should be in the past")
	assert.Equal(t, time.UTC, bootTime.Location(), "Boot time should be in UTC")
	t.Logf("Boot Time: %s", bootTime.Format(time.RFC3339))
}

// TestBootID tests that BootID() returns a string.
func TestBootID(t *testing.T) {
	bootID := BootID()
	// Boot ID may be empty on some systems (like macOS)
	t.Logf("Boot ID: %s", bootID)
}

// TestMachineID tests that MachineID() returns a string.
func TestMachineID(t *testing.T) {
	machineID := MachineID()
	t.Logf("Machine ID: %s", machineID)
}

// TestDmidecodeUUID tests that DmidecodeUUID() returns a string.
func TestDmidecodeUUID(t *testing.T) {
	uuid := DmidecodeUUID()
	// May be empty if dmidecode is not available or requires root
	t.Logf("Dmidecode UUID: %s", uuid)
}

// TestVirtualizationEnv tests that VirtualizationEnv() returns a valid enum value.
func TestVirtualizationEnv(t *testing.T) {
	env := VirtualizationEnv()
	// Should be one of the valid VirtualizationEnvironment constants
	assert.NotNil(t, env, "Virtualization environment should not be nil")
	t.Logf("Virtualization Environment: %v", env)
}

// TestSystemManufacturer tests that SystemManufacturer() returns a string.
func TestSystemManufacturer(t *testing.T) {
	manufacturer := SystemManufacturer()
	// May be empty if dmidecode is not available or requires root
	t.Logf("System Manufacturer: %s", manufacturer)
}

// TestOSMachineID tests that OSMachineID() returns a string.
func TestOSMachineID(t *testing.T) {
	osMachineID := OSMachineID()
	t.Logf("OS Machine ID: %s", osMachineID)
}

// TestOSName tests that OSName() returns a string.
func TestOSName(t *testing.T) {
	osName := OSName()
	t.Logf("OS Name: %s", osName)
}

// TestSystemUUID tests that SystemUUID() returns a string.
func TestSystemUUID(t *testing.T) {
	uuid := SystemUUID()
	t.Logf("System UUID: %s", uuid)
}

// TestGettersConsistency tests that calling getters multiple times returns the same values.
func TestGettersConsistency(t *testing.T) {
	// These values are set at init and should never change during runtime
	arch1 := Arch()
	arch2 := Arch()
	assert.Equal(t, arch1, arch2, "Arch() should return consistent values")

	cores1 := CPULogicalCores()
	cores2 := CPULogicalCores()
	assert.Equal(t, cores1, cores2, "CPULogicalCores() should return consistent values")

	bootTime1 := BootTimeUnixSeconds()
	bootTime2 := BootTimeUnixSeconds()
	assert.Equal(t, bootTime1, bootTime2, "BootTimeUnixSeconds() should return consistent values")
}
