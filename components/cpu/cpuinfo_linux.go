//go:build linux
// +build linux

package cpu

import (
	"errors"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/host"
)

func getInfo() (Info, error) {
	arch, err := host.KernelArch()
	if err != nil {
		return Info{}, err
	}

	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return Info{}, err
	}
	cpuInfo, err := fs.CPUInfo()
	if err != nil {
		return Info{}, err
	}
	if len(cpuInfo) == 0 {
		return Info{}, errors.New("no cpu info found")
	}

	return Info{
		Arch:      arch,
		CPU:       cpuInfo[0].ModelName,
		Family:    cpuInfo[0].CPUFamily,
		Model:     cpuInfo[0].Model,
		ModelName: cpuInfo[0].ModelName,
	}, nil
}

func getCores() (Cores, error) {
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return Cores{}, err
	}
	cpuInfo, err := fs.CPUInfo()
	if err != nil {
		return Cores{}, err
	}
	return Cores{
		Logical: len(cpuInfo),
	}, nil
}
