package host

import (
	"context"
	"runtime"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/shirou/gopsutil/v4/host"
)

var (
	currentHostID              string
	currentArch                string
	currentKernelVersion       string
	currentPlatform            string
	currentPlatformFamily      string
	currentPlatformVersion     string
	currentBootTimeUnixSeconds uint64
	currentBootID              string
	currentMachineID           string
	currentDmidecodeUUID       string
	currentVirtEnv             VirtualizationEnvironment
	currentSystemManufacturer  string
	currentOSMachineID         string
)

func init() {
	loadInfo()
}

func loadInfo() {
	var err error
	currentHostID, err = host.HostID()
	if err != nil {
		log.Logger.Errorw("failed to get host id", "error", err)
	}
	currentArch, err = host.KernelArch()
	if err != nil {
		log.Logger.Errorw("failed to get kernel arch", "error", err)
	}
	currentKernelVersion, err = host.KernelVersion()
	if err != nil {
		log.Logger.Errorw("failed to get kernel version", "error", err)
	}
	currentPlatform, currentPlatformFamily, currentPlatformVersion, err = host.PlatformInformation()
	if err != nil {
		log.Logger.Errorw("failed to get platform information", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	currentBootTimeUnixSeconds, err = host.BootTimeWithContext(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get boot time", "error", err)
	}

	if runtime.GOOS != "linux" {
		return
	}

	currentBootID, err = GetBootID()
	if err != nil {
		log.Logger.Errorw("failed to get boot id", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentMachineID, err = GetMachineID(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get machine id", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentDmidecodeUUID, err = GetDmidecodeUUID(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get UUID from dmidecode", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentVirtEnv, err = GetSystemdDetectVirt(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to detect virtualization environment", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentSystemManufacturer, err = GetSystemManufacturer(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to detect virtualization environment", "error", err)
	}

	currentOSMachineID, err = ReadOSMachineID()
	if err != nil {
		log.Logger.Errorw("failed to get machine id", "error", err)
	}
}

func HostID() string {
	return currentHostID
}

func Arch() string {
	return currentArch
}

func KernelVersion() string {
	return currentKernelVersion
}

func Platform() string {
	return currentPlatform
}

func PlatformFamily() string {
	return currentPlatformFamily
}

func PlatformVersion() string {
	return currentPlatformVersion
}

func BootTimeUnixSeconds() uint64 {
	return currentBootTimeUnixSeconds
}

func BootID() string {
	return currentBootID
}

func MachineID() string {
	return currentMachineID
}

func DmidecodeUUID() string {
	return currentDmidecodeUUID
}

func VirtualizationEnv() VirtualizationEnvironment {
	return currentVirtEnv
}

func SystemManufacturer() string {
	return currentSystemManufacturer
}

func OSMachineID() string {
	return currentOSMachineID
}
