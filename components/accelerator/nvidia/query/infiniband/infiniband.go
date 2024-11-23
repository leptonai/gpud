// Package infiniband provides utilities to query infiniband status.
package infiniband

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/log"
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

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()

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

		select {
		case err := <-p.Wait():
			if err != nil {
				return 0, err
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return count, serr
		}
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			return count, err
		}
	case <-ctx.Done():
		return count, ctx.Err()
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

func RunIbstat(ctx context.Context) (*IbstatOutput, error) {
	p, err := exec.LookPath("ibstat")
	if err != nil {
		return nil, fmt.Errorf("ibstat not found (%w)", err)
	}
	b, err := exec.CommandContext(ctx, p).CombinedOutput()
	if err != nil {
		return nil, err
	}
	o := &IbstatOutput{
		Raw: string(b),
	}

	// TODO: once stable return error
	o.Parsed, err = ParseIBStat(o.Raw)
	if err != nil {
		// TODO: once stable return error
		log.Logger.Errorw("failed to parse ibstat output", "error", err)

		// fallback to old ibstat checks
		if err := ValidateIbstatOutput(o.Raw); err != nil {
			o.Errors = append(o.Errors, err.Error())
		}
	}

	return o, nil
}

var (
	ErrIbstatOutputBrokenStateDown        = errors.New("ibstat output unexpected; found State: Down (check the physical switch)")
	ErrIbstatOutputBrokenPhysicalDisabled = errors.New("ibstat output unexpected; found Physical state: Disabled (check the physical switch)")
)

func ValidateIbstatOutput(s string) error {
	if strings.Contains(s, "State: Down") {
		return ErrIbstatOutputBrokenStateDown
	}

	// needs
	// "ip link set <dev> up"
	if strings.Contains(s, "Physical state: Disabled") {
		return ErrIbstatOutputBrokenPhysicalDisabled
	}

	return nil
}

type IbstatOutput struct {
	Parsed IBStatCards `json:"parsed,omitempty"`
	Raw    string      `json:"raw"`
	Errors []string    `json:"errors,omitempty"`
}
