package infiniband

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

var (
	defaultFlapAutoClearWindowMu sync.RWMutex

	// defaultFlapAutoClearWindowValue is the opt-in recovery window applied to IB
	// port flap events. When set to a positive duration, a flapping port that
	// has since been stably recovered (thresholds passing) for longer than the
	// window stops being surfaced, so the node can auto-recover without manual
	// intervention.
	//
	// This is the flap analog of the drop sticky window (defaultDropStickyWindow):
	// both keep the component Unhealthy for a stabilization period after the ports
	// recover, then clear automatically. The difference is the default — drops use
	// a 10m window out of the box, whereas flaps default to 0, which preserves the
	// historical behavior of staying surfaced (Unhealthy) until an operator runs
	// `gpud set-healthy`. Operators opt into drop-like auto-recovery by setting a
	// positive window via --infiniband-flap-auto-clear-window.
	defaultFlapAutoClearWindowValue = defaultFlapAutoClearWindow
)

// GetDefaultFlapAutoClearWindow returns the current IB port flap auto-clear window.
func GetDefaultFlapAutoClearWindow() time.Duration {
	defaultFlapAutoClearWindowMu.RLock()
	defer defaultFlapAutoClearWindowMu.RUnlock()
	return defaultFlapAutoClearWindowValue
}

// SetDefaultFlapAutoClearWindow configures the IB port flap auto-clear window.
// A value <= 0 keeps the historical "flaps are sticky until set-healthy" behavior.
func SetDefaultFlapAutoClearWindow(window time.Duration) {
	log.Logger.Infow("setting default infiniband flap auto-clear window", "window", window)

	defaultFlapAutoClearWindowMu.Lock()
	defer defaultFlapAutoClearWindowMu.Unlock()
	defaultFlapAutoClearWindowValue = window
}
