package customplugins

import (
	"fmt"
	"sort"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// ExecuteInOrder executes all the plugins in the specs, in sequence.
// This is ONLY used for dry-run plugins from the spec.
// If failFast is true, the execution will stop at the first failed plugin.
func (specs Specs) ExecuteInOrder(gpudInstance *components.GPUdInstance, failFast bool) ([]components.CheckResult, error) {
	// execute "init" type plugins first
	sort.Slice(specs, func(i, j int) bool {
		// "init" type first
		if specs[i].PluginType == "init" && specs[j].PluginType == "init" {
			return i < j
		}
		return specs[i].PluginType == "init"
	})

	results := make([]components.CheckResult, 0, len(specs))
	for _, spec := range specs {
		initFunc := spec.NewInitFunc()
		if initFunc == nil {
			continue
		}

		comp, err := initFunc(gpudInstance)
		if err != nil {
			return nil, err
		}

		checkResult := comp.Check()
		_ = comp.Close()

		if checkResult.HealthStateType() != apiv1.HealthStateTypeHealthy {
			if failFast {
				return nil, fmt.Errorf("plugin %s returned unhealthy state (summary %s)", comp.Name(), checkResult.Summary())
			}
			log.Logger.Warnw("plugin returned unhealthy state", "component", comp.Name())
		} else {
			log.Logger.Infow("executed custom plugin", "component", comp.Name())
		}

		results = append(results, checkResult)
	}
	return results, nil
}
