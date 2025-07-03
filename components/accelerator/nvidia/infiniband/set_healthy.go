package infiniband

import (
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")

	// TODO: implement

	return nil
}
