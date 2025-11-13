package sxid

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for sxid")

	if c.eventBucket != nil {
		now := c.getTimeNowFunc()
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix())
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged sxid events", "count", purged)
	}

	// Immediately update current state to reflect the purge
	// This matches the old behavior where SetHealthy event triggered immediate state update
	if err := c.updateCurrentState(); err != nil {
		log.Logger.Errorw("failed to update current state after purge", "error", err)
		return err
	}

	return nil
}
