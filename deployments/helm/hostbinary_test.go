package helm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

// hostBinaryStub stands in for GPUd. "update" rewrites the stub's own version
// line, the way GPUd replaces its own executable. Like real releases, versions
// before v0.14.1 do not read GPUD_TOKEN, and versions before v0.14.4 reject
// --recovery-token-file and --instance-id; "-notoken" versions model any other
// binary without GPUD_TOKEN.
const hostBinaryStub = `#!/bin/sh
# Model a node directory that cannot run programs (a noexec mount).
case "$0" in "${GPUD_TEST_NOEXEC_DIR:-/nonexistent}"/*) exit 126 ;; esac
VERSION=@VERSION@
case "${1:-}" in
--version)
  echo "gpud version $VERSION"
  exit 0
  ;;
up)
  case "$VERSION" in
  v0.13.* | v0.14.0 | v0.14.0-* | *-notoken) ;;
  *) echo GPUD_TOKEN ;;
  esac
  exit 0
  ;;
update)
  printf '%s\n' "$*" >> "$GPUD_TEST_DIR/update"
  printf '%s %s\n' "$(lsb_release -i -s 2>/dev/null)" "$(lsb_release -r -s 2>/dev/null)" > "$GPUD_TEST_DIR/lsb_release"
  next=""
  for arg in "$@"; do
    case "$arg" in --next-version=*) next="${arg#--next-version=}" ;; esac
  done
  # The wrapper must always pass a version: without one, GPUd picks the
  # release track from its own version instead of the stable track.
  [ -n "$next" ] || exit 3
  if [ "${GPUD_TEST_UPDATE_FAIL:-}" = true ]; then
    # Leave a changed binary behind, so the test shows that the wrapper
    # restores the image binary after a failed install.
    next=v0.0.0-partial
  fi
  sed "s/^VERSION=.*/VERSION=$next/" "$0" > "$0.new"
  chmod 0755 "$0.new"
  mv -f "$0.new" "$0"
  [ "${GPUD_TEST_UPDATE_FAIL:-}" != true ] || exit 1
  exit 0
  ;;
esac
case "$VERSION" in
v0.13.* | v0.14.[0-3] | v0.14.[0-3]-*)
  for arg in "$@"; do
    case "$arg" in
    --recovery-token-file=* | --instance-id=*)
      echo "flag provided but not defined: ${arg%%=*}" >&2
      exit 1
      ;;
    esac
  done
  ;;
esac
printf '%s\n' "$0" > "$GPUD_TEST_DIR/exec"
printf '%s\n' "$@" > "$GPUD_TEST_DIR/argv"
command -v lsb_release > "$GPUD_TEST_DIR/exec-lsb_release" || true
`

// curlStub serves the stable_latest.txt lookup.
const curlStub = `#!/bin/sh
printf '%s\n' "$*" >> "$GPUD_TEST_DIR/curl"
[ "${GPUD_TEST_CURL_FAIL:-}" != true ] || exit 22
printf '%s\n' "${GPUD_TEST_LATEST:-0.98.0}"
`

func writeHostBinaryStub(t *testing.T, path, version string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	stub := strings.Replace(hostBinaryStub, "@VERSION@", version, 1)
	require.NoError(t, os.WriteFile(path, []byte(stub), mode))
}

func stubVersion(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^VERSION=(\S+)$`).FindSubmatch(b)
	require.NotNil(t, m, "no version in %s", path)
	return string(m[1])
}

func renderDaemonSet(t *testing.T, set ...string) (appsv1.DaemonSet, error) {
	t.Helper()
	args := []string{"template", "gpud", "gpud", "--show-only", "templates/daemonset.yaml"}
	for _, s := range set {
		args = append(args, "--set", s)
	}
	// Keep stderr out of the YAML: helm writes warnings there.
	out, err := exec.Command("helm", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return appsv1.DaemonSet{}, &renderError{string(exitErr.Stderr)}
		}
		return appsv1.DaemonSet{}, err
	}
	var ds appsv1.DaemonSet
	require.NoError(t, yaml.Unmarshal(out, &ds))
	return ds, nil
}

type renderError struct{ out string }

func (e *renderError) Error() string { return e.out }

func TestHostBinaryStartup(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	// A real lsb_release on PATH, as on Ubuntu CI runners, means the wrapper
	// does not need its own.
	_, lsbErr := exec.LookPath("lsb_release")
	expectShim := lsbErr != nil

	const imageVersion = "v0.15.0"
	for _, tt := range []struct {
		name string
		set  []string
		// existing is the node copy before the start: "" for none, "broken" for
		// a copy that cannot run, or a version.
		existing   string
		stateFile  bool
		updateFail bool
		noexec     bool
		env        []string
		// labels is the node label file that the label exporter writes.
		labels string

		wantArg         string
		wantCurl        bool
		wantHostVersion string
		wantImageExec   bool
		wantSelfUpdate  bool
		wantUpdate      string
		wantLog         string
	}{
		{
			name:            "new node keeps the image version by default",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
		},
		{
			name:            "new node installs the latest stable release",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			wantCurl:        true,
			wantHostVersion: "v0.98.0",
			wantSelfUpdate:  true,
			wantUpdate:      "update --next-version=v0.98.0",
		},
		{
			name:            "new node skips the download when the image is already the latest",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			env:             []string{"GPUD_TEST_LATEST=v0.15.0"},
			wantCurl:        true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "the image binary is already GPUd v0.15.0",
		},
		{
			name:            "new node keeps the image when the latest stable release is older",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			env:             []string{"GPUD_TEST_LATEST=0.13.12"},
			wantCurl:        true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "the latest stable release v0.13.12 is older than the image binary v0.15.0; keeping the image binary",
		},
		{
			name:            "new node keeps the image binary when the latest release cannot be read",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			env:             []string{"GPUD_TEST_CURL_FAIL=true"},
			wantCurl:        true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "could not read the latest stable GPUd release; keeping the image binary",
		},
		{
			name:            "new node rejects a malformed latest release",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			env:             []string{"GPUD_TEST_LATEST=<html>$(id)"},
			wantCurl:        true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "could not read the latest stable GPUd release; keeping the image binary",
		},
		{
			name:            "new node installs a pinned release",
			set:             []string{"gpud.hostBinary.newNodeVersion=v0.15.1"},
			wantHostVersion: "v0.15.1",
			wantSelfUpdate:  true,
			wantUpdate:      "update --next-version=v0.15.1",
		},
		{
			name:            "new node skips a pinned release below the minimum version",
			set:             []string{"gpud.hostBinary.newNodeVersion=v0.14.0"},
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "GPUd v0.14.0 is older than the minimum version v0.14.1; keeping the image binary",
		},
		{
			name:            "new node restores the image binary when the download fails",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			updateFail:      true,
			wantCurl:        true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantUpdate:      "update --next-version=v0.98.0",
			wantLog:         "could not install GPUd v0.98.0; keeping the image binary",
		},
		{
			name:            "node that ran GPUd before hostBinary keeps the image version",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			stateFile:       true,
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "GPUd ran on this node before gpud.hostBinary was enabled; keeping the image binary v0.15.0",
		},
		{
			name:            "existing older copy is kept across an image bump",
			set:             []string{"gpud.hostBinary.newNodeVersion=latest"},
			existing:        "v0.14.5",
			stateFile:       true,
			wantHostVersion: "v0.14.5",
			wantSelfUpdate:  true,
		},
		{
			name:            "existing newer copy is kept",
			existing:        "v0.16.0",
			wantHostVersion: "v0.16.0",
			wantSelfUpdate:  true,
		},
		{
			name:            "copy below the chart minimum is replaced with the image binary",
			existing:        "v0.13.12",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.13.12 is older than the minimum version v0.14.1; installing the image binary v0.15.0",
		},
		{
			name:            "copy that does not read GPUD_TOKEN is replaced with the image binary",
			existing:        "v0.16.0-notoken",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.16.0-notoken does not read GPUD_TOKEN; installing the image binary",
		},
		{
			name:            "minVersion replaces an older copy with the image binary",
			set:             []string{"gpud.hostBinary.minVersion=v0.14.8"},
			existing:        "v0.14.5",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.14.5 is older than the minimum version v0.14.8; installing the image binary v0.15.0",
		},
		{
			name:            "minVersion below the chart minimum does not lower it",
			set:             []string{"gpud.hostBinary.minVersion=v0.13.0"},
			existing:        "v0.14.0",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.14.0 is older than the minimum version v0.14.1; installing the image binary v0.15.0",
		},
		{
			name:            "pod identity recovery replaces a copy that predates --recovery-token-file",
			set:             []string{"gpud.podIdentityRecovery.enabled=true"},
			existing:        "v0.14.3",
			wantArg:         "--recovery-token-file=/var/run/gpud-recovery/token",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.14.3 is older than the minimum version v0.14.4; installing the image binary v0.15.0",
		},
		{
			name:            "pod identity recovery keeps a copy that reads --recovery-token-file",
			set:             []string{"gpud.podIdentityRecovery.enabled=true"},
			existing:        "v0.14.4",
			wantArg:         "--recovery-token-file=/var/run/gpud-recovery/token",
			wantHostVersion: "v0.14.4",
			wantSelfUpdate:  true,
		},
		{
			name:            "new node skips a pinned release that predates --recovery-token-file",
			set:             []string{"gpud.podIdentityRecovery.enabled=true", "gpud.hostBinary.newNodeVersion=v0.14.3"},
			wantArg:         "--recovery-token-file=/var/run/gpud-recovery/token",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "GPUd v0.14.3 is older than the minimum version v0.14.4; keeping the image binary",
		},
		{
			name:            "instance ID label replaces a copy that predates --instance-id",
			set:             []string{"nodeLabelExporter.enabled=true"},
			env:             []string{"GPUD_INSTANCE_ID_LABEL_KEY=example.com/instance-id"},
			labels:          "example.com/instance-id=i-0123\n",
			existing:        "v0.14.3",
			wantArg:         "--instance-id=i-0123",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "v0.14.3 is older than the minimum version v0.14.4; installing the image binary v0.15.0",
		},
		{
			name:            "instance ID label key without the label keeps an older copy",
			set:             []string{"nodeLabelExporter.enabled=true"},
			env:             []string{"GPUD_INSTANCE_ID_LABEL_KEY=example.com/instance-id"},
			labels:          "example.com/other=value\n",
			existing:        "v0.14.3",
			wantHostVersion: "v0.14.3",
			wantSelfUpdate:  true,
		},
		{
			name:            "minVersion keeps a newer copy",
			set:             []string{"gpud.hostBinary.minVersion=0.14.2"},
			existing:        "v0.14.5",
			wantHostVersion: "v0.14.5",
			wantSelfUpdate:  true,
		},
		{
			name:            "minVersion newer than the image keeps the copy",
			set:             []string{"gpud.hostBinary.minVersion=v0.16.0"},
			existing:        "v0.14.5",
			wantHostVersion: "v0.14.5",
			wantSelfUpdate:  true,
			wantLog:         "the image binary v0.15.0 is older than the minimum version v0.16.0; keeping v0.14.5",
		},
		{
			name:            "broken copy is replaced with the image binary",
			existing:        "broken",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  true,
			wantLog:         "cannot run " + "@HOST@" + "; installing the image binary",
		},
		{
			name:           "noexec node runs the image binary with self-update off",
			existing:       "v0.16.0",
			noexec:         true,
			wantImageExec:  true,
			wantSelfUpdate: false,
			wantLog:        "running the image binary with self-update off",
		},
		{
			name:           "new noexec node runs the image binary with self-update off",
			set:            []string{"gpud.hostBinary.newNodeVersion=latest"},
			noexec:         true,
			wantCurl:       true,
			wantImageExec:  true,
			wantSelfUpdate: false,
			wantLog:        "running the image binary with self-update off",
		},
		{
			name:            "reinstallFromImage replaces the copy with self-update off",
			set:             []string{"gpud.hostBinary.reinstallFromImage=true"},
			existing:        "v0.16.0",
			wantHostVersion: imageVersion,
			wantSelfUpdate:  false,
			wantLog:         "gpud.hostBinary.reinstallFromImage is set",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			set := append([]string{
				"gpud.hostBinary.enabled=true",
				"gpud.enableAutoUpdate=true",
				"gpud.autoUpdateExitCode=42",
				"gpud.tokenSecret.name=enrollment",
			}, tt.set...)
			ds, err := renderDaemonSet(t, set...)
			require.NoError(t, err)
			container := ds.Spec.Template.Spec.Containers[0]
			require.Len(t, container.Command, 3)

			dir := t.TempDir()
			imageBin := filepath.Join(dir, "image", "gpud")
			hostBin := filepath.Join(dir, "host", "bin", "gpud")
			stateFile := filepath.Join(dir, "host", "gpud.state")
			shimDir := filepath.Join(dir, "shims")
			osRelease := filepath.Join(dir, "os-release")
			require.NoError(t, os.WriteFile(osRelease, []byte("ID=ubuntu\nVERSION_ID=\"22.04\"\n"), 0o600))
			writeHostBinaryStub(t, imageBin, imageVersion, 0o755)
			switch tt.existing {
			case "":
			case "broken":
				// Cannot run, like a truncated copy.
				writeHostBinaryStub(t, hostBin, "v0.16.0", 0o644)
			default:
				writeHostBinaryStub(t, hostBin, tt.existing, 0o755)
			}
			if tt.stateFile {
				require.NoError(t, os.MkdirAll(filepath.Dir(stateFile), 0o755))
				require.NoError(t, os.WriteFile(stateFile, nil, 0o600))
			}

			script := container.Command[2]
			for old, repl := range map[string]string{
				"GPUD_IMAGE_BIN=/usr/local/bin/gpud\n":       "GPUD_IMAGE_BIN=" + imageBin + "\n",
				"GPUD_HOST_BIN=/var/lib/gpud/bin/gpud\n":     "GPUD_HOST_BIN=" + hostBin + "\n",
				"GPUD_STATE_FILE=/var/lib/gpud/gpud.state\n": "GPUD_STATE_FILE=" + stateFile + "\n",
				"GPUD_SHIM_DIR=/tmp/gpud-shims\n":            "GPUD_SHIM_DIR=" + shimDir + "\n",
			} {
				require.Equal(t, 1, strings.Count(script, old))
				script = strings.Replace(script, old, repl, 1)
			}
			if tt.labels != "" {
				labelFile := filepath.Join(dir, "node-labels.env")
				require.NoError(t, os.WriteFile(labelFile, []byte(tt.labels), 0o600))
				old := `LABEL_FILE="/run/gpud-node-labels/node-labels.env"`
				require.Equal(t, 1, strings.Count(script, old))
				script = strings.Replace(script, old, `LABEL_FILE="`+labelFile+`"`, 1)
			}
			require.GreaterOrEqual(t, strings.Count(script, "/etc/os-release"), 2, "the lsb_release shim reads /etc/os-release")
			script = strings.ReplaceAll(script, "/etc/os-release", osRelease)
			require.NotContains(t, script, "/usr/local/bin/gpud up --help")
			require.NotContains(t, script, `exec /usr/local/bin/gpud "$@"`)

			binDir := filepath.Join(dir, "path")
			require.NoError(t, os.MkdirAll(binDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(binDir, "curl"), []byte(curlStub), 0o755))

			// The image's /bin/sh is dash. Set GPUD_HELM_TEST_SHELL=dash to test
			// with it where /bin/sh is another shell.
			shell := container.Command[0]
			if s := os.Getenv("GPUD_HELM_TEST_SHELL"); s != "" {
				shell = s
			}
			cmd := exec.Command(shell, container.Command[1], script)
			cmd.Env = append([]string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"GPUD_TEST_DIR=" + dir,
				"GPUD_TOKEN=synthetic-token",
			}, tt.env...)
			if tt.updateFail {
				cmd.Env = append(cmd.Env, "GPUD_TEST_UPDATE_FAIL=true")
			}
			if tt.noexec {
				cmd.Env = append(cmd.Env, "GPUD_TEST_NOEXEC_DIR="+filepath.Dir(hostBin))
			}
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			if tt.wantLog != "" {
				require.Contains(t, string(output), strings.ReplaceAll(tt.wantLog, "@HOST@", hostBin))
			}

			execPath, err := os.ReadFile(filepath.Join(dir, "exec"))
			require.NoError(t, err)
			wantExec := hostBin
			if tt.wantImageExec {
				wantExec = imageBin
			}
			require.Equal(t, wantExec+"\n", string(execPath))
			require.Contains(t, string(output), "Starting: "+wantExec+" run (token=<set>)")
			if tt.wantHostVersion != "" {
				require.Equal(t, tt.wantHostVersion, stubVersion(t, hostBin))
			}
			require.Equal(t, imageVersion, stubVersion(t, imageBin), "the image binary must never change")

			argv, err := os.ReadFile(filepath.Join(dir, "argv"))
			require.NoError(t, err)
			args := strings.Split(strings.TrimSpace(string(argv)), "\n")
			require.Contains(t, args, "--version-file=")
			if tt.wantArg != "" {
				require.Contains(t, args, tt.wantArg)
			}
			if tt.wantSelfUpdate {
				require.Contains(t, args, "--enable-auto-update=true")
				require.Contains(t, args, "--auto-update-exit-code=42")
			} else {
				require.Contains(t, args, "--enable-auto-update=false")
				require.Contains(t, args, "--auto-update-exit-code=-1")
				require.NotContains(t, args, "--auto-update-exit-code=42")
			}

			// GPUd finds lsb_release through PATH, both for the new-node
			// install and for later self-updates.
			execLSB, err := os.ReadFile(filepath.Join(dir, "exec-lsb_release"))
			require.NoError(t, err)
			if expectShim {
				require.Equal(t, filepath.Join(shimDir, "lsb_release")+"\n", string(execLSB))
			}

			curls, err := os.ReadFile(filepath.Join(dir, "curl"))
			if tt.wantCurl {
				require.NoError(t, err)
				require.Contains(t, string(curls), "https://pkg.gpud.dev/stable_latest.txt")
			} else {
				require.ErrorIs(t, err, os.ErrNotExist, "unexpected lookup: %s", curls)
			}

			updates, err := os.ReadFile(filepath.Join(dir, "update"))
			if tt.wantUpdate == "" {
				require.ErrorIs(t, err, os.ErrNotExist, "unexpected update: %s", updates)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantUpdate+"\n", string(updates))
				if expectShim {
					lsb, err := os.ReadFile(filepath.Join(dir, "lsb_release"))
					require.NoError(t, err)
					require.Equal(t, "ubuntu 22.04\n", string(lsb))
				}
			}

			entries, err := os.ReadDir(filepath.Dir(hostBin))
			require.NoError(t, err)
			for _, e := range entries {
				require.Equal(t, "gpud", e.Name(), "leftover temporary file")
			}
		})
	}
}

func TestHostBinaryRender(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}

	t.Run("disabled by default", func(t *testing.T) {
		ds, err := renderDaemonSet(t)
		require.NoError(t, err)
		script := ds.Spec.Template.Spec.Containers[0].Command[2]
		require.Contains(t, script, `exec /usr/local/bin/gpud "$@"`)
		require.NotContains(t, script, "GPUD_HOST_BIN")
		require.NotContains(t, script, "--version-file")
		require.NotContains(t, script, "lsb_release")
	})

	for _, tt := range []struct {
		name, set, want string
	}{
		{name: "commandOverride", set: "gpud.commandOverride[0]=/bin/true", want: "unset gpud.commandOverride"},
		{name: "newNodeVersion without v prefix", set: "gpud.hostBinary.newNodeVersion=0.14.3", want: "gpud.hostBinary.newNodeVersion must be"},
		{name: "newNodeVersion with shell syntax", set: "gpud.hostBinary.newNodeVersion=v1$(id)", want: "gpud.hostBinary.newNodeVersion must be"},
		{name: "minVersion with shell syntax", set: "gpud.hostBinary.minVersion=`id`", want: "gpud.hostBinary.minVersion must be"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderDaemonSet(t, "gpud.hostBinary.enabled=true", tt.set)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}
