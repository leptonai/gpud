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
	defaultLookbackPeriod   = DefaultLookbackPeriod
)

const (
	// DefaultRebootThreshold is the default reboot threshold.
	DefaultRebootThreshold = 2
	// DefaultLookbackPeriod is the default lookback window for XID events.
	DefaultLookbackPeriod = eventstore.DefaultRetention
)

// GetDefaultRebootThreshold returns the configured reboot threshold for XID recovery.
func GetDefaultRebootThreshold() RebootThreshold {
	defaultRebootThresholdMu.RLock()
	defer defaultRebootThresholdMu.RUnlock()
	return defaultRebootThreshold
}

// SetDefaultRebootThreshold updates the configured reboot threshold for XID recovery.
func SetDefaultRebootThreshold(threshold RebootThreshold) {
	log.Logger.Infow("setting default reboot threshold", "threshold", threshold.Threshold)

	defaultRebootThresholdMu.Lock()
	defer defaultRebootThresholdMu.Unlock()
	defaultRebootThreshold = threshold
}

// GetLookbackPeriod returns the XID event lookback window.
func GetLookbackPeriod() time.Duration {
	defaultLookbackPeriodMu.RLock()
	defer defaultLookbackPeriodMu.RUnlock()
	return defaultLookbackPeriod
}

// SetLookbackPeriod updates the XID event lookback window.
func SetLookbackPeriod(period time.Duration) {
	log.Logger.Infow("setting lookback period", "period", period)

	defaultLookbackPeriodMu.Lock()
	defer defaultLookbackPeriodMu.Unlock()
	defaultLookbackPeriod = period
}
