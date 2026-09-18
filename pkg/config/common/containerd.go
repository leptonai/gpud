package common

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultContainerdEndpoint    = "unix:///run/containerd/containerd.sock"
	DefaultContainerdConfigPath  = "/etc/containerd/config.toml"
	DefaultContainerdServiceName = "containerd"
)

// ContainerdConfig selects one runtime installation. The zero value preserves
// legacy runtime detection; an explicit configuration must never fall back to
// another runtime when the selected runtime is unavailable.
type ContainerdConfig struct {
	// Use unix:///run/k3s/containerd/containerd.sock for RKE2/k3s.
	Endpoint string `json:"endpoint,omitempty"`
	// Use /var/lib/rancher/rke2/agent/etc/containerd/config.toml for RKE2.
	ConfigPath string `json:"config_path,omitempty"`
	// Use rke2-agent for RKE2 workers, or rke2-server for RKE2 servers.
	ServiceName string `json:"service_name,omitempty"`
	// Optional command prefix, e.g. nsenter --target 1 --mount -- systemctl.
	// Both activeness and uptime must query the same service manager.
	SystemctlCommands string `json:"systemctl_commands,omitempty"`
}

func (c ContainerdConfig) IsZero() bool { return c == (ContainerdConfig{}) }

func (c ContainerdConfig) WithDefaults() ContainerdConfig {
	if c.Endpoint == "" {
		c.Endpoint = DefaultContainerdEndpoint
	}
	if c.ConfigPath == "" {
		c.ConfigPath = DefaultContainerdConfigPath
	}
	if c.ServiceName == "" {
		c.ServiceName = DefaultContainerdServiceName
	}
	return c
}

var containerdServiceNameRE = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.@:-]*$`)

// Validate checks syntax only: the socket may be absent during a runtime outage.
func (c ContainerdConfig) Validate() error {
	c = c.WithDefaults()
	if _, err := c.SocketPath(); err != nil {
		return err
	}
	if !filepath.IsAbs(c.ConfigPath) || filepath.Clean(c.ConfigPath) == "/" {
		return fmt.Errorf("containerd config path must be an absolute file path: %q", c.ConfigPath)
	}
	if !containerdServiceNameRE.MatchString(c.ServiceName) {
		return fmt.Errorf("invalid containerd service name: %q", c.ServiceName)
	}
	return nil
}

func (c ContainerdConfig) SocketPath() (string, error) {
	endpoint := c.WithDefaults().Endpoint
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid containerd endpoint: %w", err)
	}
	if !strings.HasPrefix(endpoint, "unix:///") || strings.ContainsAny(endpoint, "%?#\r\n") || u.Scheme != "unix" || u.Host != "" || u.User != nil || u.Opaque != "" || !filepath.IsAbs(u.Path) || filepath.Clean(u.Path) == "/" {
		return "", fmt.Errorf("containerd endpoint must be unix:///absolute/socket/path: %q", c.Endpoint)
	}
	return u.Path, nil
}
