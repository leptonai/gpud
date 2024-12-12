// Package infiniband provides utilities to query infiniband status.
package infiniband

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns true if the product supports infiniband.
// e.g.,
// "NVIDIA A100"
// "NVIDIA H100"
func SupportsInfinibandProduct(gpuProductName string) bool {
	p := strings.ToLower(gpuProductName)
	return strings.Contains(p, "a100") || strings.Contains(p, "h100")
}

// Returns the default non-zero per-port rate in GB/sec if the product supports infiniband.
func SupportsInfinibandPortRate(gpuProductName string) int {
	p := strings.ToLower(gpuProductName)
	if strings.Contains(p, "a100") {
		return 200
	}
	if strings.Contains(p, "h100") {
		return 400
	}
	return 0
}

func IbstatExists() bool {
	p, err := exec.LookPath("ibstat")
	if err != nil {
		return false
	}
	return p != ""
}

// lspci | grep -i infiniband
// 1a:00.0 Infiniband controller: Mellanox Technologies MT2910 Family [ConnectX-7]
// 3c:00.0 Infiniband controller: Mellanox Technologies MT2910 Family [ConnectX-7]
// 1a:00.0 Ethernet controller: Mellanox Technologies MT2910 Family [ConnectX-7]
// 1b:00.0 Ethernet controller: Mellanox Technologies MT2892 Family [ConnectX-6 Dx]
func CountInfinibandPCIBuses(ctx context.Context) (int, error) {
	lspciPath, err := file.LocateExecutable("lspci")
	if err != nil {
		return 0, nil
	}
	if lspciPath == "" {
		return 0, nil
	}

	p, err := process.New(
		process.WithCommand(lspciPath),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return 0, err
	}

	if err := p.Start(ctx); err != nil {
		return 0, err
	}

	count := 0

	if err := process.ReadAllStdout(
		ctx,
		p,
		process.WithProcessLine(func(line string) {
			switch {
			// e.g.,
			// 1a:00.0 Infiniband controller: Mellanox Technologies MT2910 Family [ConnectX-7]
			// 3c:00.0 Infiniband controller: Mellanox Technologies MT2910 Family [ConnectX-7]
			case strings.Contains(strings.ToLower(line), "infiniband"),

				// 1a:00.0 Ethernet controller: Mellanox Technologies MT2910 Family [ConnectX-7]
				// 1b:00.0 Ethernet controller: Mellanox Technologies MT2892 Family [ConnectX-6 Dx]
				strings.Contains(strings.ToLower(line), "mellanox"),

				strings.Contains(strings.ToLower(line), "qlogic"):
				count++
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return count, err
	}

	return count, nil
}

// Counts the directories in "/sys/class/infiniband".
// Returns 0 if the directory does not exist.
func CountInfinibandClass() int {
	info, err := os.Stat("/sys/class/infiniband")
	if err != nil || !info.IsDir() {
		return 0
	}
	dirs, err := os.ReadDir("/sys/class/infiniband")
	if err != nil {
		return 0
	}
	return len(dirs)
}

func countInfinibandClass(dir string) int {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return 0
	}
	dirs, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	return len(dirs)
}
