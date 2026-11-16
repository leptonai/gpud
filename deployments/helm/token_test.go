package helm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

func TestEnrollmentTokenStartup(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	for _, tt := range []struct {
		name, secret, label, want string
		labels                    bool
	}{
		{name: "secret", secret: "synthetic-secret", want: "synthetic-secret"},
		{name: "label", label: "synthetic-label", want: "synthetic-label", labels: true},
		{name: "secret overrides label", secret: "synthetic-secret", label: "synthetic-label", want: "synthetic-secret", labels: true},
		{name: "empty secret falls back to label", label: "synthetic-label", want: "synthetic-label", labels: true},
		{name: "disabled label ignored", label: "synthetic-label"},
		{name: "no credential"},
		{name: "special characters", secret: "synthetic space = $ & / ' \" \\ token\nnewline", want: "synthetic space = $ & / ' \" \\ token\nnewline"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			labelFile := filepath.Join(dir, "labels")
			require.NoError(t, os.WriteFile(labelFile, []byte("gpud.dgxc.lepton.ai/token="+tt.label+"\n"), 0600))
			args := []string{"template", "gpud", "gpud", "--show-only", "templates/daemonset.yaml",
				"--set-string", "gpud.tokenSecret.name=enrollment", "--set-string", "nodeLabelExporter.outputFile=" + labelFile}
			if tt.labels {
				args = append(args, "--set", "nodeLabelExporter.enabled=true")
			}
			rendered, err := exec.Command("helm", args...).Output()
			require.NoError(t, err)
			var ds appsv1.DaemonSet
			require.NoError(t, yaml.Unmarshal(rendered, &ds))
			container := ds.Spec.Template.Spec.Containers[0]
			require.Len(t, container.Command, 3)
			// Execute the rendered wrapper with a substitute binary so assertions
			// cover both its logs and the actual arguments/environment at exec.
			binary := filepath.Join(dir, "gpud")
			require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$GPUD_TEST_DIR/argv\"\nprintf '%s' \"${GPUD_TOKEN-}\" > \"$GPUD_TEST_DIR/token\"\nprintf '%s' \"${GPUD_TOKEN+set}\" > \"$GPUD_TEST_DIR/present\"\n"), 0700))
			script := container.Command[2]
			require.Equal(t, 1, strings.Count(script, `exec /usr/local/bin/gpud "$@"`))
			script = strings.Replace(script, `exec /usr/local/bin/gpud "$@"`, `exec "$GPUD_TEST_BINARY" "$@"`, 1)
			cmd := exec.Command(container.Command[0], container.Command[1], script)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GPUD_TEST_BINARY=" + binary, "GPUD_TEST_DIR=" + dir, "GPUD_TOKEN=" + tt.secret}
			for _, env := range container.Env {
				if env.ValueFrom == nil {
					cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
				} else if env.Name == "GPUD_TOKEN" {
					require.Equal(t, "enrollment", env.ValueFrom.SecretKeyRef.Name)
				}
			}
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			argv, err := os.ReadFile(filepath.Join(dir, "argv"))
			require.NoError(t, err)
			require.NotContains(t, string(argv), "--token")
			for _, token := range []string{tt.secret, tt.label} {
				if token != "" {
					require.NotContains(t, string(output), token)
					require.NotContains(t, string(argv), token)
				}
			}
			token, err := os.ReadFile(filepath.Join(dir, "token"))
			require.NoError(t, err)
			require.Equal(t, tt.want, string(token))
			present, err := os.ReadFile(filepath.Join(dir, "present"))
			require.NoError(t, err)
			if tt.want == "" {
				require.Empty(t, present)
				require.Contains(t, string(output), "token=<unset>")
			} else {
				require.Equal(t, "set", string(present))
				require.Contains(t, string(output), "token=<set>")
			}
		})
	}
}
