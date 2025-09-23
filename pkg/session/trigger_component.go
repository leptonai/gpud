package session

import (
	"net/http"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// processTriggerComponent handles the logic for triggering component checks.
// This is extracted from processRequestAsync to maintain single responsibility.
func (s *Session) processTriggerComponent(payload Request, response *Response) {
	checkResults := make([]components.CheckResult, 0)

	if payload.ComponentName != "" {
		// requesting a specific component, tag is ignored
		comp := s.componentsRegistry.Get(payload.ComponentName)
		if comp == nil {
			log.Logger.Warnw("component not found", "name", payload.ComponentName)
			response.ErrorCode = http.StatusNotFound
			return
		}

		log.Logger.Infow("triggering component check via triggerComponent", "component", payload.ComponentName)
		checkResults = append(checkResults, comp.Check())
	} else if payload.TagName != "" {
		components := s.componentsRegistry.All()
		for _, comp := range components {
			matched := false
			for _, tag := range comp.Tags() {
				if tag == payload.TagName {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			log.Logger.Infow("triggering component check via triggerComponent", "component", payload.ComponentName)
			checkResults = append(checkResults, comp.Check())
		}
	}

	response.States = apiv1.GPUdComponentHealthStates{}
	for _, checkResult := range checkResults {
		response.States = append(response.States, apiv1.ComponentHealthStates{
			Component: checkResult.ComponentName(),
			States:    checkResult.HealthStates(),
		})
	}
}
