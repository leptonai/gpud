package sxid

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

var (
	defaultLookbackPeriodMu sync.RWMutex
	defaultLookbackPeriod   = DefaultLookbackPeriod
)

const (
	// DefaultLookbackPeriod is the default lookback window for SXID events.
	DefaultLookbackPeriod = eventstore.DefaultRetention
)

func GetLookbackPeriod() time.Duration {
	defaultLookbackPeriodMu.RLock()
	defer defaultLookbackPeriodMu.RUnlock()
	return defaultLookbackPeriod
}

func SetLookbackPeriod(period time.Duration) {
	log.Logger.Infow("setting lookback period", "period", period)

	defaultLookbackPeriodMu.Lock()
	defer defaultLookbackPeriodMu.Unlock()
	defaultLookbackPeriod = period
}
