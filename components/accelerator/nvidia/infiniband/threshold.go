package infiniband

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = infiniband.ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() infiniband.ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states infiniband.ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}
