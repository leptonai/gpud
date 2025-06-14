package infiniband

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
)

// IBPort is the port of the IB card.
type IBPort struct {
	// Device is the name of the IB port (e.g., mlx5_1).
	Device string `json:"device,omitempty"`
	// State is the state of the IB port (e.g., "Active", "Down")
	State string `json:"state,omitempty"`
	// PhysicalState is the physical state of the IB port (e.g., "LinkUp", "Disabled", "Polling")
	PhysicalState string `json:"physical_state,omitempty"`
	// Rate is the rate of the IB port (e.g., 400)
	Rate int `json:"rate,omitempty"`
}

// checkPortsAndRate returns all [IBPort]s that match the expected thresholds.
// The specified rate is the threshold for "Port 1"."Rate", where it evaluates with ">=" operator
// (e.g., count all the cards whose rate is >= 400).
//
// If the `expectedPhysicalState` is empty, it matches all states.
// If the `expectedPhysicalState` are multiple states, it matches all states with OR operator.
// If the `expectedState` is empty, it matches all states.
// If the `atLeastRate` is 0, it ignores the rate check.
func checkPortsAndRate(ports []IBPort, expectedPhysicalStates []string, expectedState string, atLeastRate int) (matched []IBPort) {
	expStates := make(map[string]struct{})
	for _, s := range expectedPhysicalStates {
		expStates[s] = struct{}{}
	}

	for _, card := range ports {
		// e.g.,
		// expected "Physical state: LinkUp"
		// but got "Physical state: Disabled" or "Physical state: Polling"
		_, found := expStates[card.PhysicalState]
		if len(expStates) > 0 && !found {
			continue
		}

		// e.g.,
		// expected "State: Active"
		// but got "State: Down"
		if expectedState != "" && card.State != expectedState {
			continue
		}

		// only check if atLeastRate is specified
		if atLeastRate > 0 && atLeastRate > card.Rate {
			// does NOT meet the expected rate threshold
			// thus should not be counted
			continue
		}

		matched = append(matched, card)
	}
	return matched
}

// EvaluatePortsAndRate checks if the number of active IB port devices matches expectations.
//
// It returns a map whose key is the device name and value is the IB port,
// which does not satisfy the thresholds. It returns nil, if all the IB port devices
// satisfy the thresholds, or thresholds are not specified.
//
// It returns an error, if and only if the number of active IB ports that are >= atLeastRate
// is less than the expected number of ports (lower than the thresholds).
func EvaluatePortsAndRate(allPorts []IBPort, atLeastPorts int, atLeastRate int) ([]IBPort, error) {
	if atLeastPorts == 0 && atLeastRate == 0 {
		return nil, nil
	}

	// select all "up" devices, and count the ones that match the expected rate with ">="
	portsWithLinkUp := checkPortsAndRate(allPorts, []string{"LinkUp"}, "", atLeastRate)
	if len(portsWithLinkUp) >= atLeastPorts {
		return nil, nil
	}

	// some ports are down or having degraded rates
	errMsg := fmt.Sprintf("only %d port(s) are active and >=%d Gb/s, expect >=%d port(s)", len(portsWithLinkUp), atLeastRate, atLeastPorts)
	log.Logger.Warnw(errMsg, "totalPorts", len(allPorts), "atLeastPorts", atLeastPorts, "atLeastRateGbPerSec", atLeastRate)

	portsWithDisabledOrPolling := checkPortsAndRate(allPorts, []string{"Disabled", "Polling"}, "", 0) // atLeastRate is ignored
	if len(portsWithDisabledOrPolling) > 0 {
		physicalStates := make(map[string][]string)
		for _, port := range portsWithDisabledOrPolling {
			physicalStates[port.PhysicalState] = append(physicalStates[port.PhysicalState], port.Device)
		}

		// some ports must be missing -- construct error message accordingly
		msgs := make([]string, 0)
		for state, names := range physicalStates {
			msgs = append(msgs, fmt.Sprintf("%d device(s) found %s (%s)", len(names), state, strings.Join(names, ", ")))
		}
		sort.Strings(msgs)

		errMsg += fmt.Sprintf("; %s", strings.Join(msgs, "; "))
	}

	return portsWithDisabledOrPolling, errors.New(errMsg)
}
