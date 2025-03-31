package host

import (
	"context"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/shirou/gopsutil/v4/host"
)

var (
	currentHostID          string
	currentArch            string
	currentKernelVersion   string
	currentPlatform        string
	currentPlatformFamily  string
	currentPlatformVersion string
	currentBootTime        uint64
)

func init() {
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
	defer cancel()
	currentBootTime, err = host.BootTimeWithContext(ctx)
	if err != nil {
		log.Logger.Errorw("failed to get boot time", "error", err)
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

func CurrentBootTime() uint64 {
	return currentBootTime
}
