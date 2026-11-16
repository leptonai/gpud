package command

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/cmd/gpud/common"
)

func TestEnrollmentTokenEnvironment(t *testing.T) {
	for _, name := range []string{"run", "up"} {
		for _, tt := range []struct {
			name  string
			args  []string
			want  string
			unset bool
			empty bool
		}{
			{name: "absent environment skips login", unset: true},
			{name: "empty environment skips login", empty: true},
			{name: "environment", want: "synthetic-enrollment-token"},
			{name: "explicit flag wins", args: []string{"--token=explicit-token"}, want: "explicit-token"},
			{name: "explicit empty wins", args: []string{"--token="}, want: ""},
		} {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				t.Setenv("GPUD_TOKEN", "synthetic-enrollment-token")
				if tt.empty {
					t.Setenv("GPUD_TOKEN", "")
				}
				if tt.unset {
					require.NoError(t, os.Unsetenv("GPUD_TOKEN"))
				}
				app := App()
				called := false
				for i := range app.Commands {
					if app.Commands[i].Name == name {
						app.Commands[i].Action = func(c *cli.Context) error {
							called = true
							token, err := common.ConsumeEnrollmentToken(c)
							require.NoError(t, err)
							require.Equal(t, tt.want, token)
							require.Equal(t, len(tt.args) > 0, c.IsSet("token"))
							_, present := os.LookupEnv("GPUD_TOKEN")
							require.False(t, present, "child processes must not inherit the credential")
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
			require.Contains(t, output.String(), "GPUD_TOKEN", "the chart probes help for environment-token support")
		})
	}
}
