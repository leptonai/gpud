package infiniband

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")

	now := c.getTimeNowFunc()

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

		// insert after purge
		setHealthyEvent := eventstore.Event{
			Component: Name,
			Time:      now,
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
		}

		cctx, cancel = context.WithTimeout(c.ctx, 10*time.Second)
		found, err := c.eventBucket.Find(cctx, setHealthyEvent)
		cancel()
		if err != nil {
			return err
		}
		if found != nil {
			log.Logger.Infow("infiniband set healthy event already exists, skipping")
			return nil
		}

		cctx, cancel = context.WithTimeout(c.ctx, 10*time.Second)
		err = c.eventBucket.Insert(cctx, setHealthyEvent)
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("infiniband set healthy event inserted")
	}

	return nil
}
