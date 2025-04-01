package host

import (
	"testing"
)

func TestInit(t *testing.T) {
	hostID := HostID()
	if hostID == "" {
		t.Error("CurrentHostID() returned empty string")
	}
}

func TestCurrentArch(t *testing.T) {
	arch := Arch()
	if arch == "" {
		t.Error("CurrentArch() returned empty string")
	}
}

func TestCurrentKernelVersion(t *testing.T) {
	kernelVersion := KernelVersion()
	if kernelVersion == "" {
		t.Error("CurrentKernelVersion() returned empty string")
	}
}

func TestCurrentPlatform(t *testing.T) {
	platform := Platform()
	if platform == "" {
		t.Error("CurrentPlatform() returned empty string")
	}
}

func TestCurrentPlatformFamily(t *testing.T) {
	platformFamily := PlatformFamily()
	if platformFamily == "" {
		t.Error("CurrentPlatformFamily() returned empty string")
	}
}

func TestCurrentPlatformVersion(t *testing.T) {
	platformVersion := PlatformVersion()
	if platformVersion == "" {
		t.Error("CurrentPlatformVersion() returned empty string")
	}
}

func TestCurrentBootTime(t *testing.T) {
	bootTime := BootTimeUnixSeconds()
	if bootTime == 0 {
		t.Error("CurrentBootTime() returned 0")
	}
}
