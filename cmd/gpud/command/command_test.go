package command

import (
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

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

func TestAppRunAndUpRefreshSessionTokenDefaultsToTrue(t *testing.T) {
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
		require.NoError(t, refreshFlag.Apply(set))
		ctx := cli.NewContext(app, set, nil)
		require.True(t, ctx.Bool("refresh-session-token"))

		require.NoError(t, set.Parse([]string{"--refresh-session-token=false"}))
		ctx = cli.NewContext(app, set, nil)
		require.False(t, ctx.Bool("refresh-session-token"))
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
