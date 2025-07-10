package infiniband

import (
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")

	if c.ibPortsStore != nil {
		// past events will be discarded
		now := c.getTimeNowFunc()
		if err := c.ibPortsStore.Tombstone(now); err != nil {
			log.Logger.Warnw("error setting tombstone", "error", err)
		} else {
			log.Logger.Infow("tombstone set", "timestamp", now)
		}
	}

	return nil
}
