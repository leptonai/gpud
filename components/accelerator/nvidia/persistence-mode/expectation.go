package persistencemode

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// ExpectedMode configures how the component evaluates persistence mode.
type ExpectedMode string

const (
	// ExpectedModeEnabled requires persistence mode to be enabled on every GPU
	// that supports it. This is the default and preserves the historical
	// behavior.
	ExpectedModeEnabled ExpectedMode = "enabled"

	// ExpectedModeAny records the persistence mode state but never reports
	// unhealthy for disabled persistence mode. Intended for GPU
	// Operator-managed clusters where no component enables persistence mode
	// and long-running GPU clients (DCGM, device plugin) already keep the
	// driver initialized, making PM-off functionally benign there (LEP-6440,
	// verified on aws-iad-nkxdev-1 with GPU Operator 26.3.2: all 8 H100s on
	// both p5.48xlarge nodes reported supported=true, enabled=false, and
	// neither the driver DaemonSet nor anything else enables it).
	ExpectedModeAny ExpectedMode = "any"
)

var (
	defaultExpectedModeMu sync.RWMutex
	defaultExpectedMode   = ExpectedModeEnabled
)

// GetDefaultExpectedMode returns the current default persistence mode expectation.
func GetDefaultExpectedMode() ExpectedMode {
	defaultExpectedModeMu.RLock()
	defer defaultExpectedModeMu.RUnlock()
	return defaultExpectedMode
}

// SetDefaultExpectedMode updates the default persistence mode expectation.
func SetDefaultExpectedMode(mode ExpectedMode) {
	log.Logger.Infow("setting default expected persistence mode", "expectedMode", mode)

	defaultExpectedModeMu.Lock()
	defer defaultExpectedModeMu.Unlock()
	defaultExpectedMode = mode
}
