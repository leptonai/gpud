package sxid

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// RebootThresholdOverride configures the reboot threshold for one XID/SXID code.
type RebootThresholdOverride struct {
	RebootThreshold int `json:"rebootThreshold"`
}

var (
	defaultRebootThresholdOverridesMu sync.RWMutex
	defaultRebootThresholdOverrides   = map[int]RebootThresholdOverride{}
)

// GetDefaultRebootThresholdOverrides returns the configured per-SXID reboot thresholds.
func GetDefaultRebootThresholdOverrides() map[int]RebootThresholdOverride {
	defaultRebootThresholdOverridesMu.RLock()
	defer defaultRebootThresholdOverridesMu.RUnlock()
	return cloneRebootThresholdOverrides(defaultRebootThresholdOverrides)
}

// SetDefaultRebootThresholdOverrides updates the configured per-SXID reboot thresholds.
func SetDefaultRebootThresholdOverrides(overrides map[int]RebootThresholdOverride) {
	overrides = cloneRebootThresholdOverrides(overrides)
	log.Logger.Infow("setting default sxid reboot threshold overrides", "thresholdOverrides", overrides)

	defaultRebootThresholdOverridesMu.Lock()
	defer defaultRebootThresholdOverridesMu.Unlock()
	defaultRebootThresholdOverrides = overrides
}

func cloneRebootThresholdOverrides(overrides map[int]RebootThresholdOverride) map[int]RebootThresholdOverride {
	if overrides == nil {
		return nil
	}
	ret := make(map[int]RebootThresholdOverride, len(overrides))
	for sxid, threshold := range overrides {
		ret[sxid] = threshold
	}
	return ret
}

func rebootThresholdForSXID(sxid uint64, defaultRebootThreshold int, overrides map[int]RebootThresholdOverride) int {
	sxidID, ok := intFromUint64(sxid)
	if !ok {
		return defaultRebootThreshold
	}

	if override, ok := overrides[sxidID]; ok && override.RebootThreshold > 0 {
		return override.RebootThreshold
	}
	return defaultRebootThreshold
}
