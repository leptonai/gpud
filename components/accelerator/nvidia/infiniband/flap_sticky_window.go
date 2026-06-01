package infiniband

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

var (
	defaultFlapStickyWindowMu sync.RWMutex

	// defaultFlapStickyWindowValue is the opt-in recovery window applied to IB
	// port flap events. When set to a positive duration, a flapping port that
	// has since been stably recovered (thresholds passing) for longer than the
	// window stops being surfaced, so the node can auto-recover without manual
	// intervention.
	//
	// This is the flap analogue of the drop sticky window (defaultDropStickyWindow):
	// both keep the component Unhealthy for a stabilization period after the ports
	// recover, then clear automatically. The difference is the default — drops use
	// a 10m window out of the box, whereas flaps default to 0, which preserves the
	// historical behavior of staying surfaced (Unhealthy) until an operator runs
	// `gpud set-healthy`. Operators opt into drop-like auto-recovery by setting a
	// positive window via --infiniband-flap-sticky-window.
	defaultFlapStickyWindowValue = defaultFlapStickyWindow
)

// GetDefaultFlapStickyWindow returns the current IB port flap sticky window.
func GetDefaultFlapStickyWindow() time.Duration {
	defaultFlapStickyWindowMu.RLock()
	defer defaultFlapStickyWindowMu.RUnlock()
	return defaultFlapStickyWindowValue
}

// SetDefaultFlapStickyWindow configures the IB port flap sticky window.
// A value <= 0 keeps the historical "flaps are sticky until set-healthy" behavior.
func SetDefaultFlapStickyWindow(window time.Duration) {
	log.Logger.Infow("setting default infiniband flap sticky window", "window", window)

	defaultFlapStickyWindowMu.Lock()
	defer defaultFlapStickyWindowMu.Unlock()
	defaultFlapStickyWindowValue = window
}
