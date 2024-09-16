package query

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

const peerMemModule = "nvidia_peermem"

func CheckLsmodPeermemModule(ctx context.Context) (*LsmodPeermemModuleOutput, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("requires sudo/root access to check if ib_core is using nvidia_peermem")
	}

	proc, err := process.New(
		[][]string{{"sudo lsmod"}},
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
	rd := proc.StdoutReader()

	// e.g.,
	// sudo lsmod | grep nvidia_peermem
	scanner := bufio.NewScanner(rd)
	lines := make([]string, 0, 10)
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, peerMemModule) {
			continue
		}
		lines = append(lines, line)

		select {
		case err = <-proc.Wait():
			if err != nil {
				log.Logger.Warnw("lsmod return error", "error", err)
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return nil, serr
		}
	}
	if err != nil {
		return nil, err
	}
	if perr := proc.Abort(ctx); perr != nil {
		return nil, err
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
