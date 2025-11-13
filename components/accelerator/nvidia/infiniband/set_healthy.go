package infiniband

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")

	now := c.getTimeNowFunc()

	// Clear the recovery time when SetHealthy is called
	// This ensures a fresh start for sticky window tracking.
	// When an operator explicitly marks the component as healthy (after inspecting
	// the hardware issue), we reset all sticky window state so new port drops/flaps
	// will be tracked independently without interference from previous recovery times.
	c.thresholdRecoveryTimeMu.Lock()
	c.thresholdRecoveryTime = nil
	c.thresholdRecoveryTimeMu.Unlock()

	if c.ibPortsStore != nil {
		// past events will be discarded
		if err := c.ibPortsStore.Tombstone(now); err != nil {
			log.Logger.Warnw("error setting tombstone", "error", err)
		} else {
			log.Logger.Infow("tombstone set", "timestamp", now)
		}
	}

	if c.eventBucket != nil {
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix())
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged infiniband events", "count", purged)
	}

	return nil
}
