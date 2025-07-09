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
		if err := c.ibPortsStore.Tombstone(c.getTimeNowFunc()); err != nil {
			log.Logger.Warnw("error setting tombstone", "error", err)
		}
	}

	return nil
}
