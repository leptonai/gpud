package validation

import (
	"context"
	"errors"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

var (
	// ErrInsufficientCPU indicates that the system has insufficient CPU cores.
	ErrInsufficientCPU = errors.New("insufficient CPU cores")
	// ErrInsufficientMemory indicates that the system has insufficient memory.
	ErrInsufficientMemory = errors.New("insufficient memory")
	// ErrInsufficientResources indicates that the system has insufficient CPU and memory.
	ErrInsufficientResources = errors.New("insufficient CPU and memory")
)

const (
	// MinimumLogicalCPUCores represents the minimum number of logical CPU cores
	// required to reliably run kubelet and supporting system services.
	MinimumLogicalCPUCores = 3
	// MinimumMemoryBytes represents the minimum amount of system memory required
	// for kubelet and related system services to make progress.
	MinimumMemoryBytes = 3 * 1024 * 1024 * 1024 // 3 GiB
)

// PlatformRequirements contains the observed system resources and the minimum
// thresholds required for joining the Lepton control plane.
type PlatformRequirements struct {
	// Observed resources
	LogicalCPUCores  int
	TotalMemoryBytes uint64

	// Minimum thresholds
	MinimumCPUCores int
	MinimumMemory   uint64
}

// GetPlatformRequirements retrieves the platform resource information required to
// confirm that the host meets the minimum thresholds for joining the Lepton
// platform.
func GetPlatformRequirements(ctx context.Context) (PlatformRequirements, error) {
	logicalCPUCores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return PlatformRequirements{}, fmt.Errorf("failed to fetch logical CPU cores: %w", err)
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return PlatformRequirements{}, fmt.Errorf("failed to fetch memory information: %w", err)
	}

	// otherwise, kubelet will fail to start with the following error:
	// E1003 04:44:16.749280   27927 kubelet.go:1643] "Failed to start ContainerManager" err="invalid Node Allocatable configuration. Resource \"memory\" has a reservation of {{2609905664 0} {<nil>}  BinarySI} but capacity of {{1003839488 0} {<nil>}  BinarySI}. Expected capacity >= reservation."
	req := PlatformRequirements{
		LogicalCPUCores:  logicalCPUCores,
		TotalMemoryBytes: vm.Total,
		MinimumCPUCores:  MinimumLogicalCPUCores,
		MinimumMemory:    MinimumMemoryBytes,
	}
	return req, nil
}

// Check verifies that the platform meets minimum resource requirements.
// Returns an error if CPU cores or memory is insufficient.
func (p PlatformRequirements) Check() error {
	if p.LogicalCPUCores < p.MinimumCPUCores && p.TotalMemoryBytes < p.MinimumMemory {
		return fmt.Errorf("%w: CPU cores: %d (minimum %d), memory: %s (minimum %s)",
			ErrInsufficientResources,
			p.LogicalCPUCores, p.MinimumCPUCores,
			p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
	}

	if p.LogicalCPUCores < p.MinimumCPUCores {
		return fmt.Errorf("%w: %d (minimum %d)",
			ErrInsufficientCPU, p.LogicalCPUCores, p.MinimumCPUCores)
	}

	if p.TotalMemoryBytes < p.MinimumMemory {
		return fmt.Errorf("%w: %s (minimum %s)",
			ErrInsufficientMemory,
			p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
	}

	return nil
}

// FormatMemoryHumanized returns the total memory formatted as a human-readable string.
func (p PlatformRequirements) FormatMemoryHumanized() string {
	return humanize.Bytes(p.TotalMemoryBytes)
}

// FormatMinimumMemoryHumanized returns the minimum memory formatted as a human-readable string.
func (p PlatformRequirements) FormatMinimumMemoryHumanized() string {
	return humanize.Bytes(p.MinimumMemory)
}
