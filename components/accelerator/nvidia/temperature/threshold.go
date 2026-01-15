package temperature

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

type Thresholds struct {
	// CelsiusSlowdownMargin is the minimum thermal margin (°C) before marking the GPU as degraded.
	CelsiusSlowdownMargin int32 `json:"celsius_slowdown_margin"`
}

// ThresholdCelsiusSlowdownMargin is the default thermal margin threshold.
// A value of 0 effectively disables margin-based degraded detection, since only
// margins <= 0 will trigger the check.
//
// For GB200/Blackwell GPUs, the slowdown threshold is approximately 87°C.
// A margin of 5°C corresponds to an alert at ~82°C.
// A margin of 10°C corresponds to an alert at ~77°C.
const ThresholdCelsiusSlowdownMargin int32 = 0

var (
	defaultThresholdsMU sync.RWMutex
	defaultThresholds   = Thresholds{
		CelsiusSlowdownMargin: ThresholdCelsiusSlowdownMargin,
	}
)

func GetDefaultThresholds() Thresholds {
	defaultThresholdsMU.RLock()
	defer defaultThresholdsMU.RUnlock()
	return defaultThresholds
}

func SetDefaultMarginThreshold(threshold Thresholds) {
	if threshold.CelsiusSlowdownMargin < 0 {
		log.Logger.Warnw("invalid negative temperature margin threshold, treating as 0", "degraded_celsius", threshold.CelsiusSlowdownMargin)
		threshold.CelsiusSlowdownMargin = 0
	}

	log.Logger.Infow("setting default temperature margin threshold", "degraded_celsius", threshold.CelsiusSlowdownMargin)

	defaultThresholdsMU.Lock()
	defer defaultThresholdsMU.Unlock()
	defaultThresholds = threshold
}
