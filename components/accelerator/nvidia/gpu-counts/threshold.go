package gpucounts

import (
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

// ExpectedGPUCounts configures the expected number of GPUs.
type ExpectedGPUCounts struct {
	// Count is the expected number of GPU devices.
	// If not set, it defaults to 0.
	Count int `json:"count"`
}

// IsZero returns true if the expected GPU counts are not set.
func (ec *ExpectedGPUCounts) IsZero() bool {
	if ec == nil {
		return true
	}
	return ec.Count <= 0
}

var (
	defaultExpectedGPUCountsMu sync.RWMutex
	defaultExpectedGPUCounts   = ExpectedGPUCounts{
		Count: 0,
	}
)

func GetDefaultExpectedGPUCounts() ExpectedGPUCounts {
	defaultExpectedGPUCountsMu.RLock()
	defer defaultExpectedGPUCountsMu.RUnlock()
	return defaultExpectedGPUCounts
}

func SetDefaultExpectedGPUCounts(cnt ExpectedGPUCounts) {
	log.Logger.Infow("setting default expected GPU counts", "count", cnt.Count)

	defaultExpectedGPUCountsMu.Lock()
	defer defaultExpectedGPUCountsMu.Unlock()
	defaultExpectedGPUCounts = cnt
}
