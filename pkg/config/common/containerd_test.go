package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerdConfig(t *testing.T) {
	var zero ContainerdConfig
	require.True(t, zero.IsZero())
	require.NoError(t, zero.Validate())
	resolved := zero.WithDefaults()
	require.Equal(t, DefaultContainerdEndpoint, resolved.Endpoint)
	require.Equal(t, DefaultContainerdConfigPath, resolved.ConfigPath)
	require.Equal(t, "containerd", resolved.ServiceName)
	p, err := zero.SocketPath()
	require.NoError(t, err)
	require.Equal(t, "/run/containerd/containerd.sock", p)
	for _, endpoint := range []string{"tcp://localhost:1234", "unix://host/socket", "unix:relative", "unix:///", "unix:///socket?query", "unix:///socket#fragment", "unix:///foo%2fbar", "unix:/socket"} {
		t.Run(endpoint, func(t *testing.T) { require.Error(t, (ContainerdConfig{Endpoint: endpoint}).Validate()) })
	}
	require.Error(t, (ContainerdConfig{ConfigPath: "relative.toml"}).Validate())
	require.Error(t, (ContainerdConfig{ServiceName: "--help"}).Validate())
	require.Error(t, (ContainerdConfig{ServiceName: "rke2-agent; exit 0"}).Validate())
	cfg := ContainerdConfig{Endpoint: "unix:///run/k3s/containerd/containerd.sock", ServiceName: "rke2-agent.service"}
	require.NoError(t, cfg.Validate())
	require.False(t, cfg.IsZero())
	p, err = cfg.SocketPath()
	require.NoError(t, err)
	require.Equal(t, "/run/k3s/containerd/containerd.sock", p)
}
