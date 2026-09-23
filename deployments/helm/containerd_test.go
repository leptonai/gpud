package helm

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

func TestContainerdChart(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	for _, tt := range []struct {
		name                                  string
		values                                []string
		socketDir, configDir, active, service string
	}{
		{name: "legacy defaults", socketDir: "/run/containerd", configDir: "/etc/containerd", active: "nsenter --target 1 --mount -- systemctl is-active containerd"},
		{name: "legacy empty override", values: []string{"gpud.containerdServiceActiveCommands="}, socketDir: "/run/containerd", configDir: "/etc/containerd"},
		{name: "rke2", values: []string{"gpud.containerd.endpoint=unix:///run/k3s/containerd/containerd.sock", "gpud.containerd.configPath=/var/lib/rancher/rke2/agent/etc/containerd/config.toml", "gpud.containerd.serviceName=rke2-agent"}, socketDir: "/run/k3s/containerd", configDir: "/var/lib/rancher/rke2/agent/etc/containerd", service: "rke2-agent"},
		{name: "custom active override", values: []string{"gpud.containerd.serviceName=rke2-server", "gpud.containerdServiceActiveCommands=custom-health-check"}, socketDir: "/run/containerd", configDir: "/etc/containerd", active: "custom-health-check", service: "rke2-server"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"template", "gpud", "gpud", "--show-only", "templates/daemonset.yaml"}
			for _, value := range tt.values {
				args = append(args, "--set-string", value)
			}
			out, err := exec.Command("helm", args...).Output()
			require.NoError(t, err)
			var ds appsv1.DaemonSet
			require.NoError(t, yaml.Unmarshal(out, &ds))
			pod := ds.Spec.Template.Spec
			volumes := map[string]string{}
			for _, v := range pod.Volumes {
				if v.HostPath != nil {
					volumes[v.Name] = v.HostPath.Path
				}
			}
			require.Equal(t, tt.socketDir, volumes["host-run-containerd"])
			require.Equal(t, tt.configDir, volumes["host-etc-containerd"])
			container := pod.Containers[0]
			for _, mount := range container.VolumeMounts {
				if mount.Name == "host-run-containerd" {
					require.Equal(t, tt.socketDir, mount.MountPath)
					require.Empty(t, mount.SubPath)
				}
				if mount.Name == "host-etc-containerd" {
					require.Equal(t, tt.configDir, mount.MountPath)
					require.True(t, mount.ReadOnly)
				}
			}
			env := map[string]string{}
			for _, e := range container.Env {
				env[e.Name] = e.Value
			}
			require.Equal(t, tt.active, env["GPUD_CONTAINERD_SERVICE_ACTIVE_COMMANDS"])
			require.Equal(t, tt.service, env["GPUD_CONTAINERD_SERVICE_NAME"])
			if tt.service == "" {
				require.NotContains(t, container.Command[2], "--containerd-endpoint=")
			} else {
				require.Equal(t, "nsenter --target 1 --mount -- systemctl", env["GPUD_CONTAINERD_SYSTEMCTL_COMMANDS"])
				require.Contains(t, container.Command[2], `--containerd-endpoint="$GPUD_CONTAINERD_ENDPOINT"`)
				require.Contains(t, container.Command[2], `--containerd-config-path="$GPUD_CONTAINERD_CONFIG_PATH"`)
				require.Contains(t, container.Command[2], `--containerd-service-name="$GPUD_CONTAINERD_SERVICE_NAME"`)
				require.Contains(t, container.Command[2], `--containerd-systemctl-commands="$GPUD_CONTAINERD_SYSTEMCTL_COMMANDS"`)
			}
		})
	}
}
