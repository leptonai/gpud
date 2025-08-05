package sxid

import (
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for sxid")

	setHealthyEvent := &eventstore.Event{
		Component: Name,
		Time:      c.getTimeNowFunc(),
		Name:      "SetHealthy",
		Type:      string(apiv1.EventTypeInfo),
	}

	select {
	case c.extraEventCh <- setHealthyEvent:
	default:
		log.Logger.Debugw("channel full, set healthy event skipped")
	}

	return nil
}
