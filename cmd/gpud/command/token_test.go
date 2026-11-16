package command

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestEnrollmentTokenEnvironment(t *testing.T) {
	for _, name := range []string{"run", "up"} {
		for _, tt := range []struct {
			name  string
			args  []string
			want  string
			unset bool
		}{
			{name: "absent environment skips login", unset: true},
			{name: "environment", want: "synthetic-enrollment-token"},
			{name: "explicit flag wins", args: []string{"--token=explicit-token"}, want: "explicit-token"},
			{name: "explicit empty wins", args: []string{"--token="}, want: ""},
		} {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				t.Setenv("GPUD_TOKEN", "synthetic-enrollment-token")
				if tt.unset {
					require.NoError(t, os.Unsetenv("GPUD_TOKEN"))
				}
				app := App()
				called := false
				for i := range app.Commands {
					if app.Commands[i].Name == name {
						app.Commands[i].Action = func(c *cli.Context) error {
							called = true
							require.Equal(t, tt.want, c.String("token"))
							require.Equal(t, !tt.unset, c.IsSet("token"))
							return nil
						}
					}
				}
				require.NoError(t, app.Run(append([]string{"gpud", name}, tt.args...)))
				require.True(t, called)
			})
		}
		t.Run(name+"/help does not expose environment value", func(t *testing.T) {
			t.Setenv("GPUD_TOKEN", "synthetic-enrollment-token")
			app := App()
			var output bytes.Buffer
			app.Writer = &output
			app.ErrWriter = &output
			require.NoError(t, app.Run([]string{"gpud", name, "--help"}))
			require.NotContains(t, output.String(), "synthetic-enrollment-token")
		})
	}
}
