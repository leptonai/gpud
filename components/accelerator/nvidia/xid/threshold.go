package xid

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

// Thresholds configures the XID reboot threshold policy.
type Thresholds struct {
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

	defaultThresholdsMu sync.RWMutex
	defaultThresholds   = Thresholds{
		Threshold:          DefaultRebootThreshold,
		ThresholdOverrides: defaultThresholdOverrides,
	}

	defaultLookbackPeriodMu sync.RWMutex
	defaultLookbackPeriod   = DefaultLookbackPeriod
)

// GetDefaultThresholds returns the configured threshold policy for XID recovery.
func GetDefaultThresholds() Thresholds {
	defaultThresholdsMu.RLock()
	defer defaultThresholdsMu.RUnlock()
	return cloneThresholds(defaultThresholds)
}

// SetDefaultThresholds updates the configured threshold policy for XID recovery.
func SetDefaultThresholds(thresholds Thresholds) {
	thresholds = normalizeThresholds(thresholds)
	log.Logger.Infow("setting default xid thresholds", "threshold", thresholds.Threshold, "thresholdOverrides", thresholds.ThresholdOverrides)

	defaultThresholdsMu.Lock()
	defer defaultThresholdsMu.Unlock()
	defaultThresholds = thresholds
}

func normalizeThresholds(thresholds Thresholds) Thresholds {
	thresholdOverrides := cloneThresholdOverrides(defaultThresholdOverrides)
	for xid, override := range thresholds.ThresholdOverrides {
		thresholdOverrides[xid] = override
	}
	thresholds.ThresholdOverrides = thresholdOverrides
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
	for xid, threshold := range overrides {
		ret[xid] = threshold
	}
	return ret
}

func rebootThresholdForXID(xid uint64, thresholds Thresholds) int {
	xidID, ok := intFromUint64(xid)
	if !ok {
		return thresholds.Threshold
	}

	if thresholds.ThresholdOverrides == nil {
		thresholds.ThresholdOverrides = defaultThresholdOverrides
	}
	if override, ok := thresholds.ThresholdOverrides[xidID]; ok && override.RebootThreshold > 0 {
		return override.RebootThreshold
	}
	return thresholds.Threshold
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
