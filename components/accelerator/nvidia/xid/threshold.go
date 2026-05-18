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

	// ThresholdOverrides configures per-XID reboot thresholds.
	ThresholdOverrides map[int]RebootThresholdOverride `json:"thresholdOverrides,omitempty"`
}

// RebootThresholdOverride configures the reboot threshold for one XID/SXID code.
type RebootThresholdOverride struct {
	RebootThreshold int `json:"rebootThreshold"`
}

var (
	// XID 94 is application specific and does NOT warrant system reboot as a
	// system-level repair signal: NVIDIA's XID catalog classifies it as a
	// contained error whose immediate action is restarting the affected app.
	// Critical ECC paths are still covered separately: XID 92 reports high
	// single-bit ECC rate, XID 48/95/140 cover uncorrectable or uncontained ECC,
	// and XID 63/64 plus the remapped-rows component cover row remapping. Keeping
	// this override narrow avoids masking those critical hardware signals.
	defaultRebootThresholdOverrides = map[int]RebootThresholdOverride{
		94: {RebootThreshold: 1000},
	}

	defaultRebootThresholdMu sync.RWMutex
	defaultRebootThreshold   = RebootThreshold{
		Threshold:          DefaultRebootThreshold,
		ThresholdOverrides: defaultRebootThresholdOverrides,
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
	return cloneRebootThreshold(defaultRebootThreshold)
}

// SetDefaultRebootThreshold updates the configured reboot threshold for XID recovery.
func SetDefaultRebootThreshold(threshold RebootThreshold) {
	threshold = normalizeRebootThreshold(threshold)
	log.Logger.Infow("setting default reboot threshold", "threshold", threshold.Threshold, "thresholdOverrides", threshold.ThresholdOverrides)

	defaultRebootThresholdMu.Lock()
	defer defaultRebootThresholdMu.Unlock()
	defaultRebootThreshold = threshold
}

func normalizeRebootThreshold(threshold RebootThreshold) RebootThreshold {
	thresholdOverrides := cloneRebootThresholdOverrides(defaultRebootThresholdOverrides)
	for xid, override := range threshold.ThresholdOverrides {
		thresholdOverrides[xid] = override
	}
	threshold.ThresholdOverrides = thresholdOverrides
	return threshold
}

func cloneRebootThreshold(threshold RebootThreshold) RebootThreshold {
	threshold.ThresholdOverrides = cloneRebootThresholdOverrides(threshold.ThresholdOverrides)
	return threshold
}

func cloneRebootThresholdOverrides(overrides map[int]RebootThresholdOverride) map[int]RebootThresholdOverride {
	if overrides == nil {
		return nil
	}
	ret := make(map[int]RebootThresholdOverride, len(overrides))
	for xid, threshold := range overrides {
		ret[xid] = threshold
	}
	return ret
}

func rebootThresholdForXID(xid uint64, threshold RebootThreshold) int {
	xidID, ok := intFromUint64(xid)
	if !ok {
		return threshold.Threshold
	}

	if threshold.ThresholdOverrides == nil {
		threshold.ThresholdOverrides = defaultRebootThresholdOverrides
	}
	if override, ok := threshold.ThresholdOverrides[xidID]; ok && override.RebootThreshold > 0 {
		return override.RebootThreshold
	}
	return threshold.Threshold
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
