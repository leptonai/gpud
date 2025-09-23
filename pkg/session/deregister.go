package session

import (
	"net/http"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// processDeregisterComponent handles component deregistration
func (s *Session) processDeregisterComponent(payload Request, response *Response) {
	if payload.ComponentName == "" {
		return
	}

	comp := s.componentsRegistry.Get(payload.ComponentName)
	if comp == nil {
		log.Logger.Warnw("component not found", "name", payload.ComponentName)
		response.ErrorCode = http.StatusNotFound
		return
	}

	deregisterable, ok := comp.(components.Deregisterable)
	if !ok {
		log.Logger.Warnw("component is not deregisterable, not implementing Deregisterable interface", "name", comp.Name())
		response.ErrorCode = http.StatusBadRequest
		response.Error = "component is not deregisterable"
		return
	}

	if !deregisterable.CanDeregister() {
		log.Logger.Warnw("component is not deregisterable", "name", comp.Name())
		response.ErrorCode = http.StatusBadRequest
		response.Error = "component is not deregisterable"
		return
	}

	cerr := comp.Close()
	if cerr != nil {
		log.Logger.Errorw("failed to close component", "error", cerr)
		response.Error = cerr.Error()
		return
	}

	// only deregister if the component is successfully closed
	_ = s.componentsRegistry.Deregister(payload.ComponentName)
}
