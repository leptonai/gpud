package query

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

const peerMemModule = "nvidia_peermem"

func CheckLsmodPeermemModule(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("nvidia_peermem check requires root")
	}

	b, err := exec.CommandContext(ctx, "sudo", "lsmod").CombinedOutput()
	if err != nil {
		return nil, err
	}

	// e.g.,
	// sudo lsmod | grep nvidia_peermem
	lines := make([]string, 0, 10)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, peerMemModule) {
			continue
		}
		lines = append(lines, line)
	}

	o := &LsmodPeermemModuleOutput{
		IbstatExists:          IbstatExists(),
		InfinibandClassExists: InfinibandClassExists(),
		Raw:                   strings.Join(lines, "\n"),
	}
	o.IbcoreUsingPeermemModule = HasLsmodInfinibandPeerMem(o.Raw)

	return o, nil
}

// Returns true if infiniband (ib_core module) is using nvidia_peermem.
func HasLsmodInfinibandPeerMem(lsmodOutput string) bool {
	if lsmodOutput == "" {
		return false
	}
	lines := strings.Split(lsmodOutput, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		moduleName := fields[0]
		if moduleName != "ib_core" {
			continue
		}

		usageCount := fields[2]
		dependentModules := fields[3]
		if moduleName == "ib_core" && usageCount != "0" && strings.Contains(dependentModules, peerMemModule) {
			return true
		}
	}
	return false
}

type LsmodPeermemModuleOutput struct {
	IbstatExists             bool   `json:"ibstat_exists"`
	InfinibandClassExists    bool   `json:"infiniband_class_exists"`
	Raw                      string `json:"raw"`
	IbcoreUsingPeermemModule bool   `json:"ibcore_using_peermem_module"`
}

func IsIbcoreExpected(gpuProductName string, ibstatExists bool, infinibandClassExists bool) bool {
	if !ibstatExists || !infinibandClassExists {
		return false
	}
	p := strings.ToLower(gpuProductName)
	return (strings.Contains(p, "h100") || strings.Contains(p, "a100")) && strings.Contains(p, "sxm")
}
