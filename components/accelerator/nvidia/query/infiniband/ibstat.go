package infiniband

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/log"
	pkg_file "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

var ErrNoIbstatCommand = errors.New("ibstat not found. cannot check ib state")

func GetIbstatOutput(ctx context.Context, ibstatCommands []string) (*IbstatOutput, error) {
	if len(ibstatCommands) == 0 || strings.TrimSpace(ibstatCommands[0]) == "" {
		return nil, ErrNoIbstatCommand
	}
	if _, err := pkg_file.LocateExecutable(strings.Split(ibstatCommands[0], " ")[0]); err != nil {
		return nil, ErrNoIbstatCommand
	}

	cmdOpts := []process.OpOption{
		process.WithCommand(ibstatCommands...),
	}
	if ibstatCommands[0] != "ibstat" {
		// more complicated commands (like mocked ibstat custom commands)
		cmdOpts = append(cmdOpts, process.WithRunAsBashScript())
	}

	p, err := process.New(cmdOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()
	b, err := p.StartAndWaitForCombinedOutput(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to run ibstat command: %w", err)
	}

	o := &IbstatOutput{
		Raw: string(b),
	}
	if len(strings.TrimSpace(o.Raw)) == 0 {
		log.Logger.Warnw("ibstat returned empty output", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw))
		return o, ErrIbstatOutputEmpty
	}

	o.Parsed, err = ParseIBStat(o.Raw)
	if err != nil {
		log.Logger.Warnw("failed to parse ibstat output", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw), "error", err)
	} else {
		log.Logger.Infow("ibstat parsed", "exitCode", p.ExitCode(), "rawInputSize", len(o.Raw))
	}

	return o, err
}

// CheckInfiniband checks if the infiniband ports are up and running with the expected thresholds.
func CheckInfiniband(ctx context.Context, ibstatCommand string, threshold ExpectedPortStates) error {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	ibstat, err := GetIbstatOutput(cctx, []string{ibstatCommand})
	cancel()

	if err != nil {
		log.Logger.Warnw("error getting ibstat output", "error", err)
		return err
	}

	atLeastPorts := threshold.AtLeastPorts
	atLeastRate := threshold.AtLeastRate
	return ibstat.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate)
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

type IBStatCards []IBStatCard

// Match returns the IB port names whose physical state, state, and "Port 1"."Rate" match the expected values.
// The specified rate is the threshold for "Port 1"."Rate", where it evaluates with ">=" operator
// (e.g., count all the cards whose rate is >= 400).
//
// If the `expectedPhysicalState` is empty, it matches all states.
// If the `expectedState` is empty, it matches all states.
func (cards IBStatCards) Match(expectedPhysicalState string, expectedState string, atLeastRate int) []string {
	names := make([]string, 0)
	for _, card := range cards {
		// e.g.,
		// expected "Physical state: LinkUp"
		// but got "Physical state: Disabled"
		if expectedPhysicalState != "" && card.Port1.PhysicalState != expectedPhysicalState {
			continue
		}

		// e.g.,
		// expected "State: Active"
		// but got "State: Down"
		if expectedState != "" && card.Port1.State != expectedState {
			continue
		}

		if atLeastRate > card.Port1.Rate {
			continue
		}

		names = append(names, card.Name)
	}
	return names
}

// CheckPortsAndRate checks if the number of active IB ports matches expectations
func (cards IBStatCards) CheckPortsAndRate(atLeastPorts int, atLeastRate int) error {
	if atLeastPorts == 0 && atLeastRate == 0 {
		return nil
	}

	totalPorts := len(cards)

	// select all "up" devices, and count the ones that match the expected rate with ">="
	portNamesWithLinkUp := cards.Match("LinkUp", "", atLeastRate)
	unstatisfieldCount := atLeastPorts - len(portNamesWithLinkUp)
	if unstatisfieldCount <= 0 {
		return nil
	}

	errMsg := fmt.Sprintf("not enough LinkUp ports, only %d LinkUp out of %d, expected at least %d ports and %d Gb/sec rate", len(portNamesWithLinkUp), totalPorts, atLeastPorts, atLeastRate)

	portNamesWithDisabled := cards.Match("Disabled", "", atLeastRate)
	if len(portNamesWithDisabled) > 0 {
		// some ports must be missing -- construct error message accordingly
		errMsg += fmt.Sprintf("; some ports might be down, %v Disabled devices with Rate > %v found (%v)",
			len(portNamesWithDisabled),
			atLeastRate,
			strings.Join(portNamesWithDisabled, ", "),
		)
	}

	unstatisfieldCount -= len(portNamesWithDisabled)
	if unstatisfieldCount > 0 {
		errMsg += "; some ports must be missing"
	}
	return errors.New(errMsg)
}

type IBStatCard struct {
	Name            string     `json:"CA name"`
	Type            string     `json:"CA type"`
	NumPorts        string     `json:"Number of ports"`
	FirmwareVersion string     `json:"Firmware version"`
	HardwareVersion string     `json:"Hardware version"`
	NodeGUID        string     `json:"Node GUID"`
	SystemImageGUID string     `json:"System image GUID"`
	Port1           IBStatPort `json:"Port 1"`
}

type IBStatPort struct {
	State         string `json:"State"`
	PhysicalState string `json:"Physical state"`
	Rate          int    `json:"Rate"`
	BaseLid       int    `json:"Base lid"`
	LinkLayer     string `json:"Link layer"`
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
