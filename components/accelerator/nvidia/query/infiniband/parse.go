package infiniband

import (
	"bufio"
	"strings"

	"sigs.k8s.io/yaml"
)

type IBStatCards []IBStatCard

// Counts the number of cards whose "Port 1"."Rate" is equal to or greater
// than the specified rate (e.g., count all the cards whose rate is >= 400).
// If `expectedState` is not empty, it only counts the cards whose "Port 1"."State" is equal to the expected state.
// If `expectedPhysicalState` is not empty, it only counts the cards whose "Port 1"."Physical state" is equal to the expected physical state.
func (cards IBStatCards) CountRates(rate int, expectedState string, expectedPhysicalState string) int {
	cnt := 0
	for _, card := range cards {
		if card.Port1.Rate < rate {
			continue
		}

		// e.g.,
		// State: Active
		// State: Down
		if expectedState != "" && card.Port1.State != expectedState {
			continue
		}

		// e.g.,
		// Physical state: LinkUp
		// Physical state: Disabled
		if expectedPhysicalState != "" && card.Port1.PhysicalState != expectedPhysicalState {
			continue
		}

		cnt++
	}
	return cnt
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
