// Package nvidiasmi runs the "nvidia-smi" binary to read GPU platform data
// that the NVML Go bindings (go-nvml) do not expose. The chassis serial
// number is one example: the driver API for it is nvmlDeviceGetPlatformInfo
// version 2, and go-nvml only calls version 1 of that API.
package nvidiasmi

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	pkgfile "github.com/leptonai/gpud/pkg/file"
)

// Example "nvidia-smi -q" output on NVIDIA GB200 NVL72:
//
//	GPU 0000000F:00:00.0
//	    Product Name                    : NVIDIA GB200
//	    ...
//	    Chassis Serial Number           : 3136434J5234567
var chassisSerialRegex = regexp.MustCompile(`(?mi)^\s*Chassis Serial Number\s*:\s*(\S+)\s*$`)

// GetChassisSerial returns the serial number of the chassis that contains
// the GPUs, as reported by "nvidia-smi -q" on NVL72-class systems. Support
// and operations teams use this value to find the physical hardware for
// repair or replacement. It returns an empty string when the platform does
// not report a chassis serial number.
func GetChassisSerial(ctx context.Context) (string, error) {
	p, err := pkgfile.LocateExecutable("nvidia-smi")
	if err != nil {
		return "", err
	}

	//nolint:gosec // p is an absolute path returned by LocateExecutable for the nvidia-smi binary.
	out, err := exec.CommandContext(ctx, p, "-q").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nvidia-smi -q failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return ParseChassisSerial(string(out)), nil
}

// ParseChassisSerial extracts the chassis serial number from
// "nvidia-smi -q" output. Non-NVL platforms omit the field or report "N/A";
// both cases return an empty string.
func ParseChassisSerial(out string) string {
	matches := chassisSerialRegex.FindStringSubmatch(out)
	if len(matches) < 2 {
		return ""
	}
	serial := strings.TrimSpace(matches[1])
	if serial == "" || strings.EqualFold(serial, "N/A") {
		return ""
	}
	return serial
}
