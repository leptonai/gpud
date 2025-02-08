// Package infiniband provides utilities to query infiniband status.
package infiniband

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns the default non-zero per-port rate in GB/sec if the product supports infiniband.
func SupportsInfinibandPortRate(gpuProductName string) int {
	p := strings.ToLower(gpuProductName)

	if strings.Contains(p, "a100") {
		return 200
	}
	if strings.Contains(p, "h100") {
		return 400
	}
	if strings.Contains(p, "b100") {
		return 400
	}
	if strings.Contains(p, "h200") {
		return 400
	}
	if strings.Contains(p, "b200") {
		return 400
	}

	return 0
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
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	count := 0

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
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

			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return count, fmt.Errorf("failed to read lspci output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}

	return count, nil
}

// Counts the directories in "/sys/class/infiniband".
// Returns 0 if the directory does not exist.
func CountInfinibandClass() int {
	return CountInfinibandClassBySubDir("/sys/class/infiniband")
}

// Count the sub-directories under the specified directory.
func CountInfinibandClassBySubDir(dir string) int {
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
