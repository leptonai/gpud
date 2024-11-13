package server

import (
	"fmt"

	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	"github.com/leptonai/gpud/components/dmesg"
	lepconfig "github.com/leptonai/gpud/config"
)

// componentDependencies defines which components depend on other components
var componentDependencies = map[string][]string{
	nvidia_component_error_xid_id.Name:  {dmesg.Name},
	nvidia_component_error_sxid_id.Name: {dmesg.Name},
}

func checkDependencies(config *lepconfig.Config) error {
	for component, dependencies := range componentDependencies {
		// Skip if the component is not enabled
		if _, enabled := config.Components[component]; !enabled {
			continue
		}

		// Check if all dependencies are enabled
		for _, dep := range dependencies {
			if _, ok := config.Components[dep]; !ok {
				return fmt.Errorf("%q requires %q to be enabled", component, dep)
			}
		}
	}
	return nil
}
