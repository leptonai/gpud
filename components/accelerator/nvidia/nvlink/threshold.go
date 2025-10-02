package nvlink

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

type ExpectedLinkStates struct {
	// AtLeastGPUsWithAllLinksFeatureEnabled is the expected/minimum number of GPUs with all links feature enabled.
	// This is useful to detect the following scenarios:
	// - DeviceGetNvLinkState() returns SUCCESS (not ERROR_NOT_SUPPORTED)
	// - Each link returns state != FEATURE_ENABLED (they're FEATURE_DISABLED or inactive)
	// - Thus, safe to assume that NVLink is supported (no NOT_SUPPORTED error)
	// - But ALL links are inactive/disabled
	// - (e.g., nvidia-smi returns "Unable to retrieve NVLink information as all links are inActive")
	// e.g., if set to 8 and one GPU has some nvlinks feature disabled, it will be considered as unhealthy.
	AtLeastGPUsWithAllLinksFeatureEnabled int `json:"at_least_gpus_with_all_links_feature_enabled"`
}

var (
	defaultExpectedLinkStatesMu sync.RWMutex
	defaultExpectedLinkStates   = ExpectedLinkStates{
		AtLeastGPUsWithAllLinksFeatureEnabled: 0,
	}
)

func GetDefaultExpectedLinkStates() ExpectedLinkStates {
	defaultExpectedLinkStatesMu.RLock()
	defer defaultExpectedLinkStatesMu.RUnlock()
	return defaultExpectedLinkStates
}

func SetDefaultExpectedLinkStates(states ExpectedLinkStates) {
	// Validate and sanitize negative values
	if states.AtLeastGPUsWithAllLinksFeatureEnabled < 0 {
		log.Logger.Warnw("invalid negative threshold, treating as 0",
			"at_least_gpus_with_all_links_feature_enabled", states.AtLeastGPUsWithAllLinksFeatureEnabled)
		states.AtLeastGPUsWithAllLinksFeatureEnabled = 0
	}

	log.Logger.Infow("setting default expected link states", "at_least_gpus_with_all_links_feature_enabled", states.AtLeastGPUsWithAllLinksFeatureEnabled)

	defaultExpectedLinkStatesMu.Lock()
	defer defaultExpectedLinkStatesMu.Unlock()
	defaultExpectedLinkStates = states
}

// IsZero returns true if the expected link states are not set.
// Treats non-positive values as unset to prevent malformed configs
// from silently disabling NVLink health checks.
func (s ExpectedLinkStates) IsZero() bool {
	return s.AtLeastGPUsWithAllLinksFeatureEnabled <= 0
}
