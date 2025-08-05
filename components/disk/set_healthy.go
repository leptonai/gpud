package disk

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
	log.Logger.Infow("set healthy event received for disk")

	if c.eventBucket != nil {
		now := c.getTimeNowFunc()
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix())
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged disk events", "count", purged)

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
			log.Logger.Infow("disk set healthy event already exists, skipping")
			return nil
		}

		cctx, cancel = context.WithTimeout(c.ctx, 10*time.Second)
		err = c.eventBucket.Insert(cctx, setHealthyEvent)
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("disk set healthy event inserted")
	}

	return nil
}
