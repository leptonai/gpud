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

	// ThresholdOverrides configures per-XID threshold overrides.
	ThresholdOverrides map[int]ThresholdOverride `json:"thresholdOverrides,omitempty"`
}

// ThresholdOverride configures threshold overrides for one XID code.
type ThresholdOverride struct {
	RebootThreshold int `json:"rebootThreshold"`
}

const (
	// DefaultRebootThreshold is the fallback number of reboot events gpud allows
	// for an XID before escalating from RebootSystem to HardwareInspection.
	// During XID health evaluation, gpud walks the event history, counts reboot
	// events that happen after a reboot-recoverable XID, and if the same XID is
	// still asking for RebootSystem after this threshold, gpud treats repeated
	// reboots as insufficient recovery and recommends hardware inspection.
	// Operators can override this default globally with --xid-reboot-threshold
	// and can override individual XIDs with --xid-thresholds or session
	// updateConfig thresholdOverrides.
	DefaultRebootThreshold = 2
	// DefaultLookbackPeriod is the default lookback window for XID events.
	DefaultLookbackPeriod = eventstore.DefaultRetention
)

var (
	// XID 94 is application specific and does NOT warrant system reboot as a
	// system-level repair signal: NVIDIA's XID catalog classifies it as a
	// contained error whose immediate action is restarting the affected app.
	// Critical ECC paths are still covered separately: XID 92 reports high
	// single-bit ECC rate, XID 48/95/140 cover uncorrectable or uncontained ECC,
	// and XID 63/64 plus the remapped-rows component cover row remapping. Keeping
	// this override narrow avoids masking those critical hardware signals.
	defaultThresholdOverrides = map[int]ThresholdOverride{
		94: {RebootThreshold: 1000},
	}

	defaultRebootThresholdMu sync.RWMutex
	defaultRebootThreshold   = RebootThreshold{
		Threshold:          DefaultRebootThreshold,
		ThresholdOverrides: defaultThresholdOverrides,
	}

	defaultLookbackPeriodMu sync.RWMutex
	defaultLookbackPeriod   = DefaultLookbackPeriod
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
	thresholdOverrides := cloneThresholdOverrides(defaultThresholdOverrides)
	for xid, override := range threshold.ThresholdOverrides {
		thresholdOverrides[xid] = override
	}
	threshold.ThresholdOverrides = thresholdOverrides
	return threshold
}

func cloneRebootThreshold(threshold RebootThreshold) RebootThreshold {
	threshold.ThresholdOverrides = cloneThresholdOverrides(threshold.ThresholdOverrides)
	return threshold
}

func cloneThresholdOverrides(overrides map[int]ThresholdOverride) map[int]ThresholdOverride {
	if overrides == nil {
		return nil
	}
	ret := make(map[int]ThresholdOverride, len(overrides))
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
		threshold.ThresholdOverrides = defaultThresholdOverrides
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
