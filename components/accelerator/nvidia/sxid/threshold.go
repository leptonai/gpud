package sxid

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// Thresholds configures the SXID reboot threshold policy.
type Thresholds struct {
	// ThresholdOverrides configures per-SXID threshold overrides.
	ThresholdOverrides map[int]ThresholdOverride `json:"thresholdOverrides,omitempty"`
}

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
	defaultThresholdsMu sync.RWMutex
	defaultThresholds   = Thresholds{}
)

// GetDefaultThresholds returns the configured threshold policy for SXID recovery.
func GetDefaultThresholds() Thresholds {
	defaultThresholdsMu.RLock()
	defer defaultThresholdsMu.RUnlock()
	return cloneThresholds(defaultThresholds)
}

// SetDefaultThresholds updates the configured threshold policy for SXID recovery.
func SetDefaultThresholds(thresholds Thresholds) {
	thresholds = normalizeThresholds(thresholds)
	log.Logger.Infow("setting default sxid thresholds", "thresholdOverrides", thresholds.ThresholdOverrides)

	defaultThresholdsMu.Lock()
	defer defaultThresholdsMu.Unlock()
	defaultThresholds = thresholds
}

func normalizeThresholds(thresholds Thresholds) Thresholds {
	thresholds.ThresholdOverrides = cloneThresholdOverrides(thresholds.ThresholdOverrides)
	return thresholds
}

func cloneThresholds(thresholds Thresholds) Thresholds {
	thresholds.ThresholdOverrides = cloneThresholdOverrides(thresholds.ThresholdOverrides)
	return thresholds
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

func rebootThresholdForSXID(sxid uint64, defaultRebootThreshold int, thresholds Thresholds) int {
	sxidID, ok := intFromUint64(sxid)
	if !ok {
		return defaultRebootThreshold
	}

	if override, ok := thresholds.ThresholdOverrides[sxidID]; ok && override.RebootThreshold > 0 {
		return override.RebootThreshold
	}
	return defaultRebootThreshold
}
