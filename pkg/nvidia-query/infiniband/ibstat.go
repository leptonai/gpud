package infiniband

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

var ErrNoIbstatCommand = errors.New("ibstat not found, cannot check ib state")

// GetIbstatOutput returns the output of the ibstat command.
func GetIbstatOutput(ctx context.Context, ibstatCommands []string) (*IbstatOutput, error) {
	if len(ibstatCommands) == 0 || strings.TrimSpace(ibstatCommands[0]) == "" {
		return nil, ErrNoIbstatCommand
	}
	if _, err := pkgfile.LocateExecutable(strings.Split(ibstatCommands[0], " ")[0]); err != nil {
		return nil, ErrNoIbstatCommand
	}

	cmdOpts := []process.OpOption{
		process.WithCommand(ibstatCommands...),
	}
	if ibstatCommands[0] != "ibstat" {
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
	o := &IbstatOutput{
		Raw: strings.TrimSpace(string(b)),
	}

	var parseErr error

	// still parse the partial output
	// even if the ibstat command failed
	if len(o.Raw) > 0 {
		o.Parsed, parseErr = ParseIBStat(o.Raw)
		if parseErr != nil {
			log.Logger.Warnw("failed to parse ibstat output", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw), "error", parseErr)
		} else {
			log.Logger.Infow("ibstat parsed", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw))
		}
	}

	if runErr != nil {
		return o, runErr
	}
	if parseErr != nil {
		return o, fmt.Errorf("failed to parse ibstat output: %w", parseErr)
	}
	if len(o.Raw) == 0 {
		return o, ErrIbstatOutputEmpty
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
}

type IBStatCards []IBStatCard

type IBStatCard struct {
	// Device is the name of the IB card (e.g., "mlx5_1")
	Device          string     `json:"CA name"`
	Type            string     `json:"CA type"`
	NumPorts        string     `json:"Number of ports"`
	FirmwareVersion string     `json:"Firmware version"`
	HardwareVersion string     `json:"Hardware version"`
	NodeGUID        string     `json:"Node GUID"`
	SystemImageGUID string     `json:"System image GUID"`
	Port1           IBStatPort `json:"Port 1"`
}

type IBStatPort struct {
	// State is the state of the IB port (e.g., "Active", "Down")
	State string `json:"State"`
	// PhysicalState is the physical state of the IB port (e.g., "LinkUp", "Disabled", "Polling")
	PhysicalState string `json:"Physical state"`
	// Rate is the rate of the IB port (e.g., 400)
	Rate int `json:"Rate"`
	// BaseLid is the base lid of the IB port (e.g., 1)
	BaseLid int `json:"Base lid"`
	// LinkLayer is the link layer of the IB port (e.g., "Ethernet")
	LinkLayer string `json:"Link layer"`
}

func (cards IBStatCards) IBPorts() []IBPort {
	ibports := make([]IBPort, 0)
	for _, card := range cards {
		ibports = append(ibports, IBPort{
			Device:        card.Device,
			PhysicalState: card.Port1.PhysicalState,
			State:         card.Port1.State,
			Rate:          card.Port1.Rate,
		})
	}
	return ibports
}

var (
	ErrIbstatOutputEmpty       = errors.New("ibstat returned empty output")
	ErrIbstatOutputNoCardFound = errors.New("parsed ibstat output does not contain any card")
)

// ParseIBStat parses ibstat output and returns YAML representation.
// Returns ErrIbstatOutputEmpty if the input is empty.
func ParseIBStat(input string) (IBStatCards, error) {
	if len(input) == 0 {
		return nil, ErrIbstatOutputEmpty
	}

	scanner := bufio.NewScanner(strings.NewReader(input))

	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// CA 'mlx5_9'
		// should be
		// - CA name: mlx5_9
		// with the indentation
		// but we convert them at the end
		if strings.HasPrefix(line, "CA '") && !strings.HasPrefix(strings.TrimSpace(line), "CA type:") {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "CA '")
			line = strings.TrimSuffix(line, "'")
			lines = append(lines, "- CA name: "+line)
			continue
		}

		// CA type: MT4125
		if strings.HasPrefix(strings.TrimSpace(line), "CA type:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Number of ports: 1
		if strings.HasPrefix(strings.TrimSpace(line), "Number of ports:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Firmware version: 28.39.1002
		if strings.HasPrefix(strings.TrimSpace(line), "Firmware version:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Hardware version: 0
		if strings.HasPrefix(strings.TrimSpace(line), "Hardware version:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Node GUID: 0xa088c20300e3142a
		if strings.HasPrefix(strings.TrimSpace(line), "Node GUID:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// System image GUID: 0xa088c20300e3142a
		if strings.HasPrefix(strings.TrimSpace(line), "System image GUID:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Port 1:
		if strings.HasPrefix(strings.TrimSpace(line), "Port 1:") {
			lines = append(lines, "  "+strings.TrimSpace(line))
			continue
		}

		// Port 1:
		//    State: ...
		if strings.HasPrefix(strings.TrimSpace(line), "State:") {
			lines = append(lines, "    "+strings.TrimSpace(line))
			continue
		}

		// Port 1:
		//    Physical state: ...
		if strings.HasPrefix(strings.TrimSpace(line), "Physical state:") {
			lines = append(lines, "    "+strings.TrimSpace(line))
			continue
		}

		// Port 1:
		//    Rate: ...
		if strings.HasPrefix(strings.TrimSpace(line), "Rate:") {
			lines = append(lines, "    "+strings.TrimSpace(line))
			continue
		}

		// Port 1:
		//    Link layer: ...
		if strings.HasPrefix(strings.TrimSpace(line), "Link layer:") {
			lines = append(lines, "    "+strings.TrimSpace(line))
			continue
		}
	}

	txt := strings.Join(lines, "\n")

	// Convert to YAML
	cards := IBStatCards{}
	if err := yaml.Unmarshal([]byte(txt), &cards); err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, ErrIbstatOutputNoCardFound
	}
	return cards, nil
}
