package host

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"

	"github.com/leptonai/gpud/pkg/log"
)

var (
	currentHostID              string
	currentArch                string
	currentVendorID            string
	currentCPUModelName        string
	currentCPUModel            string
	currentCPUFamily           string
	currentCPULogicalCores     int
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
	currentOSName              string
	currentSystemUUID          string
)

func init() {
	loadInfo()
}

// TODO: remove this logic after https://github.com/shirou/gopsutil/pull/1902
var armModelToModelName = map[string]string{
	"0xd4f": "Neoverse-V2",
	"0xd81": "Cortex-A720",
	"0xd82": "Cortex-X4",
	"0xd84": "Neoverse-V3",
	"0xd85": "Cortex-X925",
	"0xd87": "Cortex-A725",
	"0xd8e": "Neoverse-N3",
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	infos, err := cpu.InfoWithContext(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get cpu info", "error", err)
	}
	if len(infos) == 0 {
		log.Logger.Errorw("no cpu info found")
	} else {
		currentVendorID = infos[0].VendorID      // e.g., "AuthenticAMD"
		currentCPUModelName = infos[0].ModelName // e.g., "AMD EPYC Processor"
		currentCPUModel = infos[0].Model
		currentCPUFamily = infos[0].Family
	}

	// TODO: remove this logic after https://github.com/shirou/gopsutil/pull/1902
	if strings.Contains(strings.ToLower(currentCPUModelName), "undefined") {
		if v, ok := armModelToModelName[currentCPUModel]; ok && v != "" {
			currentCPUModel = v
			currentCPUModelName = v
		}
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	logicalCores, err := cpu.CountsWithContext(ctx, true)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to get cpu logical cores", "error", err)
	} else {
		currentCPULogicalCores = logicalCores
	}

	currentKernelVersion, err = host.KernelVersion()
	if err != nil {
		log.Logger.Errorw("failed to get kernel version", "error", err)
	}
	currentPlatform, currentPlatformFamily, currentPlatformVersion, err = host.PlatformInformation()
	if err != nil {
		log.Logger.Errorw("failed to get platform information", "error", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
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

	currentOSMachineID, err = GetOSMachineID()
	if err != nil {
		log.Logger.Errorw("failed to get machine id", "error", err)
	}

	currentOSName, err = GetOSName()
	if err != nil {
		log.Logger.Errorw("failed to get os name", "error", err)
	}

	currentSystemUUID, err = GetSystemUUID()
	if err != nil {
		log.Logger.Errorw("failed to get system uuid", "error", err)
	}
}

func HostID() string {
	return currentHostID
}

func Arch() string {
	return currentArch
}

func CPUVendorID() string {
	return currentVendorID
}

func CPUModelName() string {
	return currentCPUModelName
}

func CPUModel() string {
	return currentCPUModel
}

func CPUFamily() string {
	return currentCPUFamily
}

func CPULogicalCores() int {
	return currentCPULogicalCores
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

func OSName() string {
	return currentOSName
}

func SystemUUID() string {
	return currentSystemUUID
}
