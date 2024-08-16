package server

import (
	"fmt"

	nvidia_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	"github.com/leptonai/gpud/components/dmesg"
	lepconfig "github.com/leptonai/gpud/config"
)

func checkDependencies(config *lepconfig.Config) error {
	if _, ok := config.Components[nvidia_error_xid.Name]; ok {
		if _, ok := config.Components[dmesg.Name]; !ok {
			return fmt.Errorf("%q requires %q to be enabled", nvidia_error_xid.Name, dmesg.Name)
		}
	}
	if _, ok := config.Components[nvidia_error_sxid.Name]; ok {
		if _, ok := config.Components[dmesg.Name]; !ok {
			return fmt.Errorf("%q requires %q to be enabled", nvidia_error_sxid.Name, dmesg.Name)
		}
	}
	return nil
}
