package nvlink

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// ExpectedLinkStates configures the adjustable go-health NVLink thresholds.
type ExpectedLinkStates struct {
	MaxInactiveNVLinks   int `json:"max_inactive_nvlinks"`
	MaxUnhealthyP2PPeers int `json:"max_unhealthy_p2p_peers"`
}

var (
	defaultExpectedLinkStatesMu sync.RWMutex
	defaultExpectedLinkStates   ExpectedLinkStates
)

// GetDefaultExpectedLinkStates returns the process-wide default NVLink thresholds.
func GetDefaultExpectedLinkStates() ExpectedLinkStates {
	defaultExpectedLinkStatesMu.RLock()
	defer defaultExpectedLinkStatesMu.RUnlock()
	return defaultExpectedLinkStates
}

// SetDefaultExpectedLinkStates updates the process-wide default NVLink thresholds.
func SetDefaultExpectedLinkStates(states ExpectedLinkStates) {
	if states.MaxInactiveNVLinks < 0 {
		log.Logger.Warnw("invalid negative threshold, treating as 0", "max_inactive_nvlinks", states.MaxInactiveNVLinks)
		states.MaxInactiveNVLinks = 0
	}
	if states.MaxUnhealthyP2PPeers < 0 {
		log.Logger.Warnw("invalid negative threshold, treating as 0", "max_unhealthy_p2p_peers", states.MaxUnhealthyP2PPeers)
		states.MaxUnhealthyP2PPeers = 0
	}

	log.Logger.Infow("setting default expected link states", "thresholds", states)

	defaultExpectedLinkStatesMu.Lock()
	defer defaultExpectedLinkStatesMu.Unlock()
	defaultExpectedLinkStates = states
}
