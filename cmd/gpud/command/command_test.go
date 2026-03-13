package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

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
