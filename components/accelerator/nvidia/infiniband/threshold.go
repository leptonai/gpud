package infiniband

import (
	"sync"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"
)

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = types.ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

// GetDefaultExpectedPortStates returns the current default InfiniBand threshold configuration.
func GetDefaultExpectedPortStates() types.ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

// SetDefaultExpectedPortStates updates the default InfiniBand threshold configuration.
func SetDefaultExpectedPortStates(states types.ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}
