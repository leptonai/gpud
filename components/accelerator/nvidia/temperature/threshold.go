package temperature

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

type MarginThreshold struct {
	// DegradedCelsius is the minimum thermal margin (°C) before marking the GPU as degraded.
	DegradedCelsius int32 `json:"degraded_celsius"`
}

// DefaultMarginThresholdCelsius is the default thermal margin threshold.
// A value of 0 effectively disables margin-based degraded detection, since only
// margins <= 0 will trigger the check. Set a positive value (e.g., 10) via
// --temperature-margin-threshold-celsius to enable proactive thermal monitoring.
const DefaultMarginThresholdCelsius int32 = 0

var (
	defaultMarginThresholdMu sync.RWMutex
	defaultMarginThreshold   = MarginThreshold{
		DegradedCelsius: DefaultMarginThresholdCelsius,
	}
)

func GetDefaultMarginThreshold() MarginThreshold {
	defaultMarginThresholdMu.RLock()
	defer defaultMarginThresholdMu.RUnlock()
	return defaultMarginThreshold
}

func SetDefaultMarginThreshold(threshold MarginThreshold) {
	if threshold.DegradedCelsius < 0 {
		log.Logger.Warnw("invalid negative temperature margin threshold, treating as 0", "degraded_celsius", threshold.DegradedCelsius)
		threshold.DegradedCelsius = 0
	}

	log.Logger.Infow("setting default temperature margin threshold", "degraded_celsius", threshold.DegradedCelsius)

	defaultMarginThresholdMu.Lock()
	defer defaultMarginThresholdMu.Unlock()
	defaultMarginThreshold = threshold
}
