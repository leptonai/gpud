package infiniband

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type IbstatusOutput struct {
	Parsed IBStatuses `json:"parsed,omitempty"`
	Raw    string     `json:"raw"`
}

var ErrNoIbstatusCommand = errors.New("ibstatus not found, cannot check ib state")

func GetIbstatusOutput(ctx context.Context, ibstatusCommands []string) (*IbstatusOutput, error) {
	if len(ibstatusCommands) == 0 || strings.TrimSpace(ibstatusCommands[0]) == "" {
		return nil, ErrNoIbstatusCommand
	}
	if _, err := pkgfile.LocateExecutable(strings.Split(ibstatusCommands[0], " ")[0]); err != nil {
		return nil, ErrNoIbstatusCommand
	}

	cmdOpts := []process.OpOption{
		process.WithCommand(ibstatusCommands...),
	}
	if ibstatusCommands[0] != "ibstatus" {
		// more complicated commands (like mocked ibstat custom commands)
		cmdOpts = append(cmdOpts, process.WithRunAsBashScript())
	}

	p, runErr := process.New(cmdOpts...)
	if runErr != nil {
		return nil, runErr
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()
	b, runErr := p.StartAndWaitForCombinedOutput(ctx)
	o := &IbstatusOutput{
		Raw: strings.TrimSpace(string(b)),
	}

	var parseErr error

	// still parse the partial output
	// even if the ibstat command failed
	if len(o.Raw) > 0 {
		o.Parsed, parseErr = ParseIBStatus(o.Raw)
		if parseErr != nil {
			log.Logger.Warnw("failed to parse ibstatus output", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw), "error", parseErr)
		} else {
			log.Logger.Infow("ibstatus parsed", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw))
		}
	}

	if runErr != nil {
		return o, runErr
	}
	if parseErr != nil {
		return o, fmt.Errorf("failed to parse ibstatus output: %w", parseErr)
	}
	if len(o.Raw) == 0 {
		return o, ErrIbstatusOutputEmpty
	}

	return o, nil
}

type IBStatuses []IBStatus

type IBStatus struct {
	Device        string `json:"device"`
	DefaultGID    string `json:"default gid"`
	DefaultLID    string `json:"default lid"`
	SMLID         string `json:"sm lid"`
	State         string `json:"state"`
	PhysicalState string `json:"phys state"`
	Rate          string `json:"rate"`
	BaseLid       string `json:"base lid"`
	LinkLayer     string `json:"link_layer"`
}

func (devs IBStatuses) IBPorts() []IBPort {
	ibports := make([]IBPort, 0)
	for _, dev := range devs {
		ibports = append(ibports, IBPort{
			Device:        dev.Device,
			State:         sanitizeIbstatusState(dev.State),
			PhysicalState: sanitizeIbstatusPhysicalState(dev.PhysicalState),
			Rate:          parseIbstatusRate(dev.Rate),
		})
	}
	return ibports
}

// "4: ACTIVE" becomes "ACTIVE"
func sanitizeIbstatusState(state string) string {
	split := strings.Split(state, ":")
	if len(split) == 2 {
		return strings.TrimSpace(split[1])
	}
	return state
}

// "5: LinkUp" becomes "LinkUp"
func sanitizeIbstatusPhysicalState(state string) string {
	split := strings.Split(state, ":")
	if len(split) == 2 {
		return strings.TrimSpace(split[1])
	}
	return state
}

// "200 Gb/sec (4X HDR)" becomes 200
func parseIbstatusRate(rate string) int {
	split := strings.Fields(strings.TrimSpace(rate))
	if len(split) > 0 {
		s := strings.TrimSpace(split[0])
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return 0
}

var (
	ErrIbstatusOutputEmpty         = errors.New("ibstatus returned empty output")
	ErrIbstatusOutputNoDeviceFound = errors.New("parsed ibstatus output does not contain any device")
)

// ParseIBStatus parses ibstatus output and returns YAML representation.
// Returns ErrIbstatusOutputEmpty if the input is empty.
func ParseIBStatus(input string) (IBStatuses, error) {
	if len(input) == 0 {
		return nil, ErrIbstatusOutputEmpty
	}

	scanner := bufio.NewScanner(strings.NewReader(input))

	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// "Infiniband device 'mlx5_0' port 1 status:"
		// becomes
		// "mlx5_0:"
		if strings.HasPrefix(line, "Infiniband device '") {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "Infiniband device '")
			line = strings.TrimSuffix(line, "' port 1 status:")
			line += ":"
			lines = append(lines, line)
			continue
		}

		// keep indentation for remaining lines
		//
		// 'state:           4: ACTIVE'
		// becomes
		// 'state:           4: ACTIVE'
		//
		// otherwise, "mapping values are not allowed in this context"
		split := strings.SplitN(line, ":", 2)
		if len(split) == 2 {
			line = split[0] + ":" + " \"" + strings.TrimSpace(split[1]) + "\""
		}

		lines = append(lines, line)
	}

	txt := strings.Join(lines, "\n")

	// Convert to YAML
	statuses := map[string]IBStatus{}
	if err := yaml.Unmarshal([]byte(txt), &statuses); err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, ErrIbstatusOutputNoDeviceFound
	}

	converted := IBStatuses{}
	for k, v := range statuses {
		v.Device = k
		converted = append(converted, v)
	}
	sort.Slice(converted, func(i, j int) bool {
		return converted[i].Device < converted[j].Device
	})

	return converted, nil
}
