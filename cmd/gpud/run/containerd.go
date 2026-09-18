package run

import (
	"github.com/urfave/cli"

	configcommon "github.com/leptonai/gpud/pkg/config/common"
)

func containerdConfigFromCLI(ctx *cli.Context) (configcommon.ContainerdConfig, error) {
	cfg := configcommon.ContainerdConfig{
		Endpoint:          ctx.String("containerd-endpoint"),
		ConfigPath:        ctx.String("containerd-config-path"),
		ServiceName:       ctx.String("containerd-service-name"),
		SystemctlCommands: ctx.String("containerd-systemctl-commands"),
	}
	return cfg, cfg.Validate()
}
