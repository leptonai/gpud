package peermem

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	"github.com/leptonai/gpud/pkg/process"
)

const peerMemModule = "nvidia_peermem"

func CheckLsmodPeermemModule(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("requires sudo/root access to check if ib_core is using nvidia_peermem")
	}

	proc, err := process.New(
		process.WithCommand("sudo lsmod"),
		process.WithRunAsBashScript(),
		process.WithRestartConfig(
			process.RestartConfig{
				OnError:  true,
				Limit:    10,
				Interval: 5 * time.Second,
			},
		))
	if err != nil {
		return nil, err
	}
	if err := proc.Start(ctx); err != nil {
		return nil, err
	}

	// e.g.,
	// sudo lsmod | grep nvidia_peermem
	lines := make([]string, 0, 10)
	if err := process.Read(
		ctx,
		proc,
		process.WithReadStdout(),
		process.WithProcessLine(func(line string) {
			s := strings.TrimSpace(line)
			if s == "" {
				return
			}
			if !strings.Contains(s, peerMemModule) {
				return
			}
			lines = append(lines, s)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, err
	}

	if perr := proc.Abort(ctx); perr != nil {
		return nil, err
	}

	o := &LsmodPeermemModuleOutput{
		IbstatExists:          infiniband.IbstatExists(),
		InfinibandClassExists: infiniband.CountInfinibandClass() > 0,
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
