package hwslowdown

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for hw slowdown")

	if c.eventBucket != nil {
		now := c.getTimeNowFunc()
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix())
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged hw slowdown events", "count", purged)
	}

	return nil
}
