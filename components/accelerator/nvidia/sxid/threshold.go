package sxid

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// ThresholdOverride configures threshold overrides for one SXID code.
type ThresholdOverride struct {
	RebootThreshold int `json:"rebootThreshold"`
}

const (
	// DefaultRebootThreshold is the fallback number of reboot events gpud allows
	// for an SXID before escalating from RebootSystem to HardwareInspection.
	// During SXID health evaluation, gpud walks the event history, counts reboot
	// events that happen after a reboot-recoverable SXID, and if the same SXID is
	// still asking for RebootSystem after this threshold, gpud treats repeated
	// reboots as insufficient recovery and recommends hardware inspection.
	// Operators can override individual SXIDs with --sxid-thresholds.
	DefaultRebootThreshold = 2
)

var (
	defaultThresholdOverridesMu sync.RWMutex
	defaultThresholdOverrides   = map[int]ThresholdOverride{}
)

// GetDefaultThresholdOverrides returns the configured per-SXID threshold overrides.
func GetDefaultThresholdOverrides() map[int]ThresholdOverride {
	defaultThresholdOverridesMu.RLock()
	defer defaultThresholdOverridesMu.RUnlock()
	return cloneThresholdOverrides(defaultThresholdOverrides)
}

// SetDefaultThresholdOverrides updates the configured per-SXID threshold overrides.
func SetDefaultThresholdOverrides(overrides map[int]ThresholdOverride) {
	overrides = cloneThresholdOverrides(overrides)
	log.Logger.Infow("setting default sxid threshold overrides", "thresholdOverrides", overrides)

	defaultThresholdOverridesMu.Lock()
	defer defaultThresholdOverridesMu.Unlock()
	defaultThresholdOverrides = overrides
}

func cloneThresholdOverrides(overrides map[int]ThresholdOverride) map[int]ThresholdOverride {
	if overrides == nil {
		return nil
	}
	ret := make(map[int]ThresholdOverride, len(overrides))
	for sxid, threshold := range overrides {
		ret[sxid] = threshold
	}
	return ret
}

func rebootThresholdForSXID(sxid uint64, defaultRebootThreshold int, overrides map[int]ThresholdOverride) int {
	sxidID, ok := intFromUint64(sxid)
	if !ok {
		return defaultRebootThreshold
	}

	if override, ok := overrides[sxidID]; ok && override.RebootThreshold > 0 {
		return override.RebootThreshold
	}
	return defaultRebootThreshold
}
