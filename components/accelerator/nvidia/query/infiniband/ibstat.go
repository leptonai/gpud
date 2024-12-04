package infiniband

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/log"
	"sigs.k8s.io/yaml"
)

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

// ParseIBStat parses ibstat output and returns YAML representation
func ParseIBStat(input string) (IBStatCards, error) {
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
	return cards, nil
}
