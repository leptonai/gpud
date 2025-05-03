package infiniband

// IBPort is the port of the IB card.
type IBPort struct {
	Device        string
	State         string
	PhysicalState string
	Rate          int
}

// CheckPortsAndRate returns the map from the physical state to each IB port names that matches the expected values.
// The specified rate is the threshold for "Port 1"."Rate", where it evaluates with ">=" operator
// (e.g., count all the cards whose rate is >= 400).
//
// If the `expectedPhysicalState` is empty, it matches all states.
// If the `expectedPhysicalState` are multiple states, it matches all states with OR operator.
// If the `expectedState` is empty, it matches all states.
func CheckPortsAndRate(ports []IBPort, expectedPhysicalStates []string, expectedState string, atLeastRate int) (map[string][]string, []string) {
	expStates := make(map[string]struct{})
	for _, s := range expectedPhysicalStates {
		expStates[s] = struct{}{}
	}

	all, names := make(map[string][]string), make([]string, 0)
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

		if atLeastRate > card.Rate {
			continue
		}

		if _, ok := all[card.PhysicalState]; !ok {
			all[card.PhysicalState] = make([]string, 0)
		}
		all[card.PhysicalState] = append(all[card.PhysicalState], card.Device)
		names = append(names, card.Device)
	}
	return all, names
}
