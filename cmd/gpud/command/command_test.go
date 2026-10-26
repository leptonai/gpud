package command

import (
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestAppWorkspaceRegistrationValidationBeforeEffects(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing token", []string{"--workspace-id", "workspace", "--node-group", "group"}, "token is empty"},
		{"blank token", []string{"--workspace-id", "workspace", "--node-group", "group", "--token", " "}, "token is empty"},
		{"empty workspace", []string{"--workspace-id="}, "--workspace-id must not be empty"},
		{"blank workspace", []string{"--workspace-id", " ", "--token", "token", "--node-group", "group"}, "workspace ID cannot be blank"},
		{"missing node group", []string{"--workspace-id", "workspace", "--token", "token"}, "node group is required"},
		{"blank node group", []string{"--workspace-id", "workspace", "--token", "token", "--node-group", " "}, "node group is required"},
		{"wildcard workspace", []string{"--workspace-id", " * ", "--token", "token", "--node-group", "group"}, "wildcard selectors"},
		{"wildcard node group", []string{"--workspace-id", "workspace", "--token", "token", "--node-group", " * "}, "wildcard selectors"},
		{"explicit machine", []string{"--workspace-id", "workspace", "--token", "token", "--node-group", "group", "--machine-id", "machine"}, "cannot be combined with --machine-id"},
		{"explicit empty machine", []string{"--workspace-id", "workspace", "--token", "token", "--node-group", "group", "--machine-id="}, "cannot be combined with --machine-id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "state")
			app := App()
			app.Writer = io.Discard
			app.ErrWriter = io.Discard
			// An invalid log level stops accepted input before discovery or systemd.
			// Invalid enrollment must fail even before logger initialization.
			args := append([]string{"gpud", "up", "--log-level", "invalid-level", "--data-dir", dataDir}, tt.args...)
			err := app.Run(args)
			require.ErrorContains(t, err, tt.want)
			require.NoDirExists(t, dataDir)
		})
	}
}

func TestAppRunRejectsWorkspaceRegistrationFlag(t *testing.T) {
	app := App()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	err := app.Run([]string{"gpud", "run", "--workspace-id", "workspace"})
	require.ErrorContains(t, err, "flag provided but not defined: -workspace-id")
}

func TestAppRunContainerdFlags(t *testing.T) {
	app := App()
	for _, cmd := range app.Commands {
		if cmd.Name != "run" {
			continue
		}
		set := flag.NewFlagSet("run", flag.ContinueOnError)
		for _, f := range cmd.Flags {
			f.Apply(set)
		}
		for _, name := range []string{"containerd-endpoint", "containerd-config-path", "containerd-service-name", "containerd-systemctl-commands"} {
			require.NotNil(t, set.Lookup(name))
			require.Empty(t, set.Lookup(name).DefValue)
		}
		ctx := cli.NewContext(app, set, nil)
		require.Zero(t, ctx.Duration("containerd-dangling-pod-grace-period"))
		require.NoError(t, set.Set("containerd-dangling-pod-grace-period", "10m"))
		require.Equal(t, 10*time.Minute, ctx.Duration("containerd-dangling-pod-grace-period"))
		require.Error(t, set.Set("containerd-dangling-pod-grace-period", "invalid"))
		require.NoError(t, set.Parse([]string{"--containerd-endpoint=unix:///run/k3s/containerd/containerd.sock", "--containerd-service-name=rke2-agent"}))
		return
	}
	t.Fatal("run command missing")
}

func TestAppRunAndUpHaveSessionProtocolFlag(t *testing.T) {
	t.Parallel()

	app := App()
	wantCommands := map[string]bool{"run": false, "up": false}
	for _, cmd := range app.Commands {
		if _, ok := wantCommands[cmd.Name]; !ok {
			continue
		}
		wantCommands[cmd.Name] = true

		var protocolFlag cli.Flag
		for _, candidate := range cmd.Flags {
			if candidate.GetName() == "session-protocol" {
				protocolFlag = candidate
				break
			}
		}
		require.NotNilf(t, protocolFlag, "command %q is missing --session-protocol", cmd.Name)

		set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
		protocolFlag.Apply(set)
		ctx := cli.NewContext(app, set, nil)
		require.Equal(t, "auto", ctx.String("session-protocol"))

		require.NoError(t, set.Parse([]string{"--session-protocol", "v2"}))
		ctx = cli.NewContext(app, set, nil)
		require.Equal(t, "v2", ctx.String("session-protocol"))
	}

	for cmdName, found := range wantCommands {
		require.Truef(t, found, "expected command %q to exist", cmdName)
	}
}

func TestAppRunAndUpRefreshSessionTokenDefaultsToFalse(t *testing.T) {
	t.Parallel()

	app := App()
	wantCommands := map[string]bool{"run": false, "up": false}
	for _, cmd := range app.Commands {
		if _, ok := wantCommands[cmd.Name]; !ok {
			continue
		}
		wantCommands[cmd.Name] = true

		var refreshFlag cli.Flag
		for _, candidate := range cmd.Flags {
			if candidate.GetName() == "refresh-session-token" {
				refreshFlag = candidate
				break
			}
		}
		require.NotNilf(t, refreshFlag, "command %q is missing --refresh-session-token", cmd.Name)

		set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
		refreshFlag.Apply(set)
		ctx := cli.NewContext(app, set, nil)
		require.False(t, ctx.Bool("refresh-session-token"))

		require.NoError(t, set.Parse([]string{"--refresh-session-token"}))
		ctx = cli.NewContext(app, set, nil)
		require.True(t, ctx.Bool("refresh-session-token"))
	}

	for cmdName, found := range wantCommands {
		require.Truef(t, found, "expected command %q to exist", cmdName)
	}
}

func TestAppRunAndUpHaveRegionFlag(t *testing.T) {
	t.Parallel()

	app := App()
	wantCommands := map[string]bool{"run": false, "up": false}
	for _, cmd := range app.Commands {
		if _, ok := wantCommands[cmd.Name]; !ok {
			continue
		}
		wantCommands[cmd.Name] = true

		var regionFlag cli.Flag
		for _, candidate := range cmd.Flags {
			if candidate.GetName() == "region" {
				regionFlag = candidate
				break
			}
		}
		require.NotNilf(t, regionFlag, "command %q is missing --region", cmd.Name)

		set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
		regionFlag.Apply(set)
		ctx := cli.NewContext(app, set, nil)
		require.Empty(t, ctx.String("region"))

		require.NoError(t, set.Parse([]string{"--region", "eu-north-1"}))
		ctx = cli.NewContext(app, set, nil)
		require.Equal(t, "eu-north-1", ctx.String("region"))
	}

	for cmdName, found := range wantCommands {
		require.Truef(t, found, "expected command %q to exist", cmdName)
	}
}

func TestAppHasInfinibandExcludeDevicesFlag(t *testing.T) {
	t.Parallel()

	app := App()

	wantCommands := map[string]bool{
		"run":            false,
		"scan":           false,
		"custom-plugins": false,
	}

	for _, cmd := range app.Commands {
		if _, ok := wantCommands[cmd.Name]; !ok {
			continue
		}

		foundFlag := false
		for _, f := range cmd.Flags {
			if f.GetName() == "infiniband-exclude-devices" {
				foundFlag = true
				break
			}
		}
		require.Truef(t, foundFlag, "command %q is missing --infiniband-exclude-devices", cmd.Name)
		wantCommands[cmd.Name] = true
	}

	for cmdName, foundCmd := range wantCommands {
		require.Truef(t, foundCmd, "expected command %q to exist", cmdName)
	}
}

func TestAppHasOutputFormatFlag(t *testing.T) {
	t.Parallel()

	app := App()

	wantCommands := map[string]bool{
		"machine-info": false,
		"metadata":     false,
	}

	for _, cmd := range app.Commands {
		if _, ok := wantCommands[cmd.Name]; !ok {
			continue
		}

		foundFlag := false
		for _, f := range cmd.Flags {
			if f.GetName() == "output-format" {
				foundFlag = true
				break
			}
		}
		require.Truef(t, foundFlag, "command %q is missing --output-format", cmd.Name)
		wantCommands[cmd.Name] = true
	}

	for cmdName, foundCmd := range wantCommands {
		require.Truef(t, foundCmd, "expected command %q to exist", cmdName)
	}
}

func TestAppUpHasNodeLabelsFlag(t *testing.T) {
	t.Parallel()

	app := App()

	foundUp := false
	for _, cmd := range app.Commands {
		if cmd.Name != "up" {
			continue
		}

		foundUp = true
		require.Contains(t, cmd.UsageText, "--node-labels")

		foundFlag := false
		for _, f := range cmd.Flags {
			if f.GetName() != "node-labels" {
				continue
			}

			foundFlag = true
			usage := nodeLabelsFlagUsage(f)
			require.Contains(t, usage, "JSON object")
			require.Contains(t, usage, "user.node.lepton.ai/")
			require.Contains(t, usage, "{}")
		}

		require.True(t, foundFlag, "up command is missing --node-labels")
	}

	require.True(t, foundUp, "expected command %q to exist", "up")
}

func nodeLabelsFlagUsage(flag cli.Flag) string {
	switch typed := flag.(type) {
	case cli.StringFlag:
		return typed.Usage
	case *cli.StringFlag:
		return typed.Usage
	default:
		return strings.TrimSpace(flag.String())
	}
}
