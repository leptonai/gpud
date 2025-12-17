package command

import (
	"testing"

	"github.com/stretchr/testify/require"
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
