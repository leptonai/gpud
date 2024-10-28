package server

import (
	"fmt"

	nvidia_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	"github.com/leptonai/gpud/components/dmesg"
	lepconfig "github.com/leptonai/gpud/config"
)

// componentDependencies defines which components depend on other components
var componentDependencies = map[string][]string{
	nvidia_error_xid.Name:  {dmesg.Name},
	nvidia_error_sxid.Name: {dmesg.Name},
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
