package xid

import (
	"sync"

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
