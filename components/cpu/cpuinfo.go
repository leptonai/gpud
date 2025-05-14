package cpu

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"

	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/shirou/gopsutil/v4/cpu"
)

const collectCPUUsagePerCPU = false

func getInfo() Info {
	return Info{
		Arch:      pkghost.Arch(),
		GoArch:    runtime.GOARCH,
		CPU:       pkghost.CPUModelName(),
		Family:    pkghost.CPUFamily(),
		Model:     pkghost.CPUModel(),
		ModelName: pkghost.CPUModelName(),
		VendorID:  pkghost.CPUVendorID(),
	}
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
	prevStat *cpu.TimesStat,
	curStat cpu.TimesStat,
	usedPct float64,
) float64 {
	usedPercent := float64(0.0)
	if prevStat == nil {
		usedPercent = usedPct
	} else {
		usedPercent = calculateBusy(*prevStat, curStat)
	}

	return usedPercent
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
