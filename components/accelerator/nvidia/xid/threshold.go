package xid

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

// RebootThreshold configures the expected reboot threshold.
type RebootThreshold struct {
	// Threshold is the expected number of reboot events within the evaluation window.
	// If not set, it defaults to 2.
	Threshold int `json:"threshold"`
}

var (
	defaultRebootThresholdMu sync.RWMutex
	defaultRebootThreshold   = RebootThreshold{
		Threshold: DefaultRebootThreshold,
	}

	defaultLookbackPeriodMu sync.RWMutex
	defaultLookbackPeriod   = eventstore.DefaultRetention
)

const (
	// DefaultRebootThreshold is the default reboot threshold.
	DefaultRebootThreshold = 2
)

func GetDefaultRebootThreshold() RebootThreshold {
	defaultRebootThresholdMu.RLock()
	defer defaultRebootThresholdMu.RUnlock()
	return defaultRebootThreshold
}

func SetDefaultRebootThreshold(threshold RebootThreshold) {
	log.Logger.Infow("setting default reboot threshold", "threshold", threshold.Threshold)

	defaultRebootThresholdMu.Lock()
	defer defaultRebootThresholdMu.Unlock()
	defaultRebootThreshold = threshold
}

func GetLookbackPeriod() time.Duration {
	defaultLookbackPeriodMu.RLock()
	defer defaultLookbackPeriodMu.RUnlock()
	return defaultLookbackPeriod
}

func SetLookbackPeriod(period time.Duration) {
	log.Logger.Infow("setting lookback period", "period", period)

	defaultLookbackPeriodMu.Lock()
	defer defaultLookbackPeriodMu.Unlock()
	defaultLookbackPeriod = period
}
