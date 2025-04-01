package host

import (
	"testing"
)

func TestInit(t *testing.T) {
	hostID := CurrentHostID()
	if hostID == "" {
		t.Error("CurrentHostID() returned empty string")
	}
}

func TestCurrentArch(t *testing.T) {
	arch := CurrentArch()
	if arch == "" {
		t.Error("CurrentArch() returned empty string")
	}
}

func TestCurrentKernelVersion(t *testing.T) {
	kernelVersion := CurrentKernelVersion()
	if kernelVersion == "" {
		t.Error("CurrentKernelVersion() returned empty string")
	}
}

func TestCurrentPlatform(t *testing.T) {
	platform := CurrentPlatform()
	if platform == "" {
		t.Error("CurrentPlatform() returned empty string")
	}
}

func TestCurrentPlatformFamily(t *testing.T) {
	platformFamily := CurrentPlatformFamily()
	if platformFamily == "" {
		t.Error("CurrentPlatformFamily() returned empty string")
	}
}

func TestCurrentPlatformVersion(t *testing.T) {
	platformVersion := CurrentPlatformVersion()
	if platformVersion == "" {
		t.Error("CurrentPlatformVersion() returned empty string")
	}
}

func TestCurrentBootTime(t *testing.T) {
	bootTime := CurrentBootTimeUnixSeconds()
	if bootTime == 0 {
		t.Error("CurrentBootTime() returned 0")
	}
}
