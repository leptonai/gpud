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
	currentDmidecodeUUID, err = DmidecodeUUID(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get UUID from dmidecode", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentVirtEnv, err = SystemdDetectVirt(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to detect virtualization environment", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	currentSystemManufacturer, err = SystemManufacturer(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to detect virtualization environment", "error", err)
	}

	currentOSMachineID, err = ReadOSMachineID()
	if err != nil {
		log.Logger.Errorw("failed to get machine id", "error", err)
	}
}

func CurrentHostID() string {
	return currentHostID
}

func CurrentArch() string {
	return currentArch
}

func CurrentKernelVersion() string {
	return currentKernelVersion
}

func CurrentPlatform() string {
	return currentPlatform
}

func CurrentPlatformFamily() string {
	return currentPlatformFamily
}

func CurrentPlatformVersion() string {
	return currentPlatformVersion
}

func CurrentBootTimeUnixSeconds() uint64 {
	return currentBootTimeUnixSeconds
}

func CurrentBootID() string {
	return currentBootID
}

func CurrentMachineID() string {
	return currentMachineID
}

func CurrentDmidecodeUUID() string {
	return currentDmidecodeUUID
}

func CurrentVirtEnv() VirtualizationEnvironment {
	return currentVirtEnv
}

func CurrentSystemManufacturer() string {
	return currentSystemManufacturer
}

func CurrentOSMachineID() string {
	return currentOSMachineID
}
