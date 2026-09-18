package run

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestContainerdConfigFromCLI(t *testing.T) {
	set := flag.NewFlagSet("run", flag.ContinueOnError)
	for _, name := range []string{"containerd-endpoint", "containerd-config-path", "containerd-service-name", "containerd-systemctl-commands"} {
		set.String(name, "", "")
	}
	ctx := cli.NewContext(nil, set, nil)
	cfg, err := containerdConfigFromCLI(ctx)
	require.NoError(t, err)
	require.True(t, cfg.IsZero())
	require.NoError(t, set.Parse([]string{
		"--containerd-endpoint=unix:///run/k3s/containerd/containerd.sock",
		"--containerd-config-path=/var/lib/rancher/rke2/agent/etc/containerd/config.toml",
		"--containerd-service-name=rke2-agent",
		"--containerd-systemctl-commands=nsenter --target 1 --mount -- systemctl",
	}))
	cfg, err = containerdConfigFromCLI(ctx)
	require.NoError(t, err)
	require.Equal(t, "unix:///run/k3s/containerd/containerd.sock", cfg.Endpoint)
	require.Equal(t, "/var/lib/rancher/rke2/agent/etc/containerd/config.toml", cfg.ConfigPath)
	require.Equal(t, "rke2-agent", cfg.ServiceName)
	require.Equal(t, "nsenter --target 1 --mount -- systemctl", cfg.SystemctlCommands)
	require.NoError(t, set.Set("containerd-endpoint", "tcp://localhost:1234"))
	_, err = containerdConfigFromCLI(ctx)
	require.Error(t, err)
}
