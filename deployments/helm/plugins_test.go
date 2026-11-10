package helm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
)

// Exercise the rendered startup command and the real plugin file lifecycle.
// The rendered path must resolve to a hostPath, and fresh registries must reload
// the saved profile. This does not substitute for a live Kubernetes restart test.
func TestPluginProfilePersistence(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	for _, byok := range []bool{false, true} {
		name := "default"
		if byok {
			name = "byok"
		}
		t.Run(name, func(t *testing.T) {
			args := []string{"template", "gpud", "gpud", "--show-only", "templates/daemonset.yaml"}
			if byok {
				args = append(args, "--set", "nodeLabelExporter.enabled=true", "--set", "gpud.requireNVIDIADriver=false",
					"--set-string", "nodeLabelExporter.labelKeys.machineId=lepton.ai/machine-id")
			}
			out, err := exec.Command("helm", args...).Output()
			require.NoError(t, err, "%s", out)
			var ds appsv1.DaemonSet
			require.NoError(t, yaml.Unmarshal(out, &ds))
			pod := ds.Spec.Template.Spec
			container := pod.Containers[0]
			require.Len(t, container.Command, 3)

			// Capture argv after the real shell wrapper has applied its defaults.
			capture := filepath.Join(t.TempDir(), "argv")
			script := container.Command[2]
			require.Equal(t, 1, strings.Count(script, `exec /usr/local/bin/gpud "$@"`))
			script = strings.Replace(script, `exec /usr/local/bin/gpud "$@"`, `printf '%s\n' "$@" > "$GPUD_TEST_ARGS"`, 1)
			cmd := exec.Command(container.Command[0], container.Command[1], script)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GPUD_TEST_ARGS=" + capture}
			for _, env := range container.Env {
				if env.ValueFrom == nil {
					cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
				}
			}
			out, err = cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			argv, err := os.ReadFile(capture)
			require.NoError(t, err)
			pluginPath := customplugins.DefaultPluginSpecsFile
			for _, arg := range strings.Split(string(argv), "\n") {
				if strings.HasPrefix(arg, "--plugin-specs-file=") {
					pluginPath = strings.TrimPrefix(arg, "--plugin-specs-file=")
				}
			}

			// Resolve the file through the rendered volume mounts, just as the
			// container runtime does. Require a writable host directory so a
			// replacement pod on this node also retains the saved profile.
			var mount *corev1.VolumeMount
			for i := range container.VolumeMounts {
				m := &container.VolumeMounts[i]
				if strings.HasPrefix(pluginPath, m.MountPath+"/") && (mount == nil || len(m.MountPath) > len(mount.MountPath)) {
					mount = m
				}
			}
			require.NotNil(t, mount, "plugin file %s is lost when the container restarts", pluginPath)
			require.False(t, mount.ReadOnly)
			require.Empty(t, mount.SubPath)
			var hostPath *corev1.HostPathVolumeSource
			for _, volume := range pod.Volumes {
				if volume.Name == mount.Name {
					hostPath = volume.HostPath
				}
			}
			require.NotNil(t, hostPath, "plugin storage must survive pod replacement")
			require.Equal(t, "/var/lib/gpud", hostPath.Path)
			require.NotNil(t, hostPath.Type)
			require.Equal(t, corev1.HostPathDirectoryOrCreate, *hostPath.Type)
			hostDir := t.TempDir()
			path := filepath.Join(hostDir, strings.TrimPrefix(pluginPath, mount.MountPath+"/"))
			require.Equal(t, hostDir, filepath.Dir(path), "parent directory must exist on a fresh node")

			profile := customplugins.Specs{{
				PluginName: "chart-persistence-" + name,
				PluginType: customplugins.SpecTypeComponent,
				RunMode:    string(apiv1.RunModeTypeManual),
				Timeout:    metav1.Duration{Duration: 30 * time.Second},
				HealthStatePlugin: &customplugins.Plugin{Steps: []customplugins.Step{{
					Name: "check", RunBashScript: &customplugins.RunBashScript{ContentType: "plaintext", Script: "exit 0"},
				}}},
			}}
			save := func(specs customplugins.Specs, wantRestart bool) {
				t.Helper()
				updated, err := customplugins.SaveSpecs(path, specs)
				require.NoError(t, err)
				require.Equal(t, wantRestart, updated, "only changed profiles should request a restart")
			}
			reload := func(want customplugins.Specs) {
				t.Helper()
				loaded, err := customplugins.LoadSpecs(path)
				require.NoError(t, err)
				require.True(t, loaded.Equal(want))
				registry := components.NewRegistry(&components.GPUdInstance{RootCtx: context.Background()})
				for _, spec := range loaded {
					registry.MustRegister(spec.NewInitFunc())
				}
				require.Len(t, registry.All(), len(want))
				for _, component := range registry.All() {
					require.NoError(t, component.Start())
					require.Equal(t, apiv1.HealthStateTypeHealthy, component.Check().HealthStateType())
					require.NoError(t, component.Close())
				}
			}

			save(nil, false) // Fresh node with no attached profile.
			save(profile, true)
			reload(profile) // New process registers and runs the manual component.
			for i := 0; i < 3; i++ {
				save(profile, false)
			}
			reload(profile) // A replacement pod sees the same host file.
			profile[0].HealthStatePlugin.Steps[0].RunBashScript.Script = "printf updated"
			save(profile, true)
			reload(profile)
			save(profile, false)
			save(nil, true) // Detach clears the profile across subsequent starts.
			reload(nil)
			save(nil, false)
			save(customplugins.Specs{}, false)
		})
	}
}
