package session

import (
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
)

// processSetHealthy handles the setHealthy request
func (s *Session) processSetHealthy(payload Request) {
	log.Logger.Infow("setHealthy received", "components", payload.Components)

	for _, componentName := range payload.Components {
		comp := s.componentsRegistry.Get(componentName)
		if comp == nil {
			log.Logger.Errorw("failed to get component", "error", errdefs.ErrNotFound)
			continue
		}
		if healthSettable, ok := comp.(components.HealthSettable); ok {
			if err := healthSettable.SetHealthy(); err != nil {
				log.Logger.Errorw("failed to set healthy", "component", componentName, "error", err)
			} else {
				log.Logger.Infow("successfully set healthy", "component", componentName)
			}
		} else {
			log.Logger.Warnw("component does not implement HealthSettable, dropping setHealthy request", "component", componentName)
		}
	}
}
