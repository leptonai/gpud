//go:build !linux
// +build !linux

package cpu

import (
	"context"
	"errors"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
)

func getInfo() (Info, error) {
	arch, err := host.KernelArch()
	if err != nil {
		return Info{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	infos, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return Info{}, err
	}
	if len(infos) == 0 {
		return Info{}, errors.New("no cpu info found")
	}

	info := Info{
		Arch:      arch,
		CPU:       infos[0].ModelName,
		Family:    infos[0].Family,
		Model:     infos[0].Model,
		ModelName: infos[0].ModelName,
	}
	return info, nil
}

func getCores() (Cores, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logicalCores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return Cores{}, err
	}
	return Cores{
		Logical: logicalCores,
	}, nil
}
