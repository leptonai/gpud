package infiniband

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"
)

// checkPortsAndRate returns all [types.IBPort]s that match the expected thresholds.
// The specified rate is the threshold for "Port 1"."Rate", where it evaluates with ">=" operator
// (e.g., count all the cards whose rate is >= 400).
//
// If the `expectedPhysicalState` is empty, it matches all states.
// If the `expectedPhysicalState` are multiple states, it matches all states with OR operator.
// If the `expectedState` is empty, it matches all states.
// If the `atLeastRate` is 0, it ignores the rate check.
func checkPortsAndRate(ports []types.IBPort, expectedPhysicalStates []string, atLeastRate int) (matched []types.IBPort) {
	expStates := make(map[string]struct{})
	for _, s := range expectedPhysicalStates {
		expStates[s] = struct{}{}
	}

	for _, port := range ports {
		if !port.IsIBPort() {
			continue
		}

		// e.g.,
		// expected "Physical state: LinkUp"
		// but got "Physical state: Disabled" or "Physical state: Polling"
		_, found := expStates[port.PhysicalState]
		if len(expStates) > 0 && !found {
			continue
		}

		// only check if atLeastRate is specified
		if atLeastRate > 0 && atLeastRate > port.RateGBSec {
			// does NOT meet the expected rate threshold
			// thus should not be counted
			continue
		}

		matched = append(matched, port)
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
func EvaluatePortsAndRate(allPorts []types.IBPort, atLeastPorts int, atLeastRate int) ([]types.IBPort, error) {
	if atLeastPorts == 0 && atLeastRate == 0 {
		return nil, nil
	}

	// maps from device name to its state
	devStates := make(map[string]string, len(allPorts))
	for _, port := range allPorts {
		devStates[port.Device] = port.State
	}

	// select all "up" devices, and count the ones that match the expected rate with ">="
	portsWithLinkUp := checkPortsAndRate(allPorts, []string{"LinkUp"}, atLeastRate)
	if len(portsWithLinkUp) >= atLeastPorts {
		return nil, nil
	}

	// some ports are down or having degraded rates
	errMsg := fmt.Sprintf("only %d port(s) are active and >=%d Gb/s, expect >=%d port(s)", len(portsWithLinkUp), atLeastRate, atLeastPorts)
	log.Logger.Warnw(errMsg, "totalPorts", len(allPorts), "atLeastPorts", atLeastPorts, "atLeastRateGbPerSec", atLeastRate)

	portsWithDisabledOrPolling := checkPortsAndRate(allPorts, []string{"Disabled", "Polling"}, 0) // atLeastRate is ignored
	if len(portsWithDisabledOrPolling) > 0 {
		physicalStates := make(map[string][]string)
		for _, port := range portsWithDisabledOrPolling {
			physicalStates[port.PhysicalState] = append(physicalStates[port.PhysicalState], port.Device)
		}

		// some ports must be missing -- construct error message accordingly
		msgs := make([]string, 0)
		for physicalState, devNames := range physicalStates {
			msg := fmt.Sprintf("%d device(s) physical state %s", len(devNames), physicalState)
			msg += " (" + strings.Join(devNames, ", ") + ")"

			switch physicalState {
			case "Polling":
				msg += " -- connecton lost from this card to other cards/switches"
			default:
			}

			msgs = append(msgs, msg)
		}
		sort.Strings(msgs)

		errMsg += fmt.Sprintf("; %s", strings.Join(msgs, "; "))
	}

	return portsWithDisabledOrPolling, errors.New(errMsg)
}
