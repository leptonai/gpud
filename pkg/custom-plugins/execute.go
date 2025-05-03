package customplugins

import (
	"sort"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// ExecuteInOrder executes all the plugins in the specs, in sequence.
// This is ONLY used for dry-run plugins from the spec.
func (specs Specs) ExecuteInOrder(gpudInstance *components.GPUdInstance) (map[string]components.CheckResult, error) {
	results := make(map[string]components.CheckResult)

	// execute "init" type plugins first
	sort.Slice(specs, func(i, j int) bool {
		// "init" type first
		if specs[i].Type == "init" && specs[j].Type == "init" {
			return i < j
		}
		return specs[i].Type == "init"
	})

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
		results[comp.Name()] = checkResult
		_ = comp.Close()
		log.Logger.Infow("executed custom plugin", "component", comp.Name())
	}
	return results, nil
}
