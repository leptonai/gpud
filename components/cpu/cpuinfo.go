package cpu

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
)

const collectCPUUsagePerCPU = false

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

var (
	prevMu sync.RWMutex
	prev   *cpu.TimesStat
)

func setPrevTimeStat(t cpu.TimesStat) {
	prevMu.Lock()
	defer prevMu.Unlock()
	prev = &t
}

func getPrevTimeStat() *cpu.TimesStat {
	prevMu.RLock()
	defer prevMu.RUnlock()
	return prev
}

func getTimeStatForAllCPUs(ctx context.Context) (cpu.TimesStat, error) {
	timeStats, err := cpu.TimesWithContext(ctx, collectCPUUsagePerCPU)
	if err != nil {
		return cpu.TimesStat{}, err
	}
	if len(timeStats) != 1 {
		return cpu.TimesStat{}, fmt.Errorf("expected 1 cpu time stat, got %d", len(timeStats))
	}
	return timeStats[0], nil
}

func getUsedPercentForAllCPUs(ctx context.Context) (float64, error) {
	usages, err := cpu.PercentWithContext(ctx, 0, collectCPUUsagePerCPU)
	if err != nil {
		return 0, err
	}
	if len(usages) != 1 {
		return 0, fmt.Errorf("expected 1 cpu usage, got %d", len(usages))
	}
	return usages[0], nil
}

// calculateCPUUsage calculates the CPU usage percentage and updates the data structure
func calculateCPUUsage(
	ctx context.Context,
	prevStat *cpu.TimesStat,
	getTimeStat func(ctx context.Context) (cpu.TimesStat, error),
	getUsedPct func(ctx context.Context) (float64, error),
) (cpu.TimesStat, float64, error) {
	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	curStat, err := getTimeStat(cctx)
	ccancel()
	if err != nil {
		return cpu.TimesStat{}, 0.0, err
	}

	usedPercent := float64(0.0)
	if prevStat == nil {
		cctx, ccancel = context.WithTimeout(ctx, 5*time.Second)
		usedPercent, err = getUsedPct(cctx)
		ccancel()
		if err != nil {
			return cpu.TimesStat{}, 0, err
		}
	} else {
		usedPercent = calculateBusy(*prevStat, curStat)
	}

	return curStat, usedPercent, nil
}

// copied from https://pkg.go.dev/github.com/shirou/gopsutil/v4/cpu#PercentWithContext
func calculateBusy(t1, t2 cpu.TimesStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2Busy-t1Busy)/(t2All-t1All)*100))
}

// copied from https://pkg.go.dev/github.com/shirou/gopsutil/v4/cpu#PercentWithContext
func getAllBusy(t cpu.TimesStat) (float64, float64) {
	// copied from "cpu.TimesStat.Total()" (deprecated)
	total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq +
		t.Softirq + t.Steal + t.Guest + t.GuestNice

	if runtime.GOOS == "linux" {
		total -= t.Guest     // Linux 2.6.24+
		total -= t.GuestNice // Linux 3.2.0+
	}

	busy := total - t.Idle - t.Iowait
	return total, busy
}
