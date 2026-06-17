package nfs

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for nfs")

	if c.eventBucket != nil {
		now := c.getTimeNow()
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix()+1)
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged nfs events", "count", purged)
	}

	return nil
}

func (c *component) getTimeNow() time.Time {
	if c != nil && c.getTimeNowFunc != nil {
		return c.getTimeNowFunc().UTC()
	}
	return time.Now().UTC()
}
