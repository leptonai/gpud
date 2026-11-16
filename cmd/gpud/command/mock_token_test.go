//go:build linux

package command

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/login"
	"github.com/leptonai/gpud/pkg/osutil"
)

func TestCommandsConsumeEnrollmentTokenBeforeStartup(t *testing.T) {
	for _, name := range []string{"run", "up"} {
		for _, tt := range []struct {
			name, env, want string
			args            []string
			login           bool
		}{
			{name: "environment", env: "synthetic-secret", want: "synthetic-secret", login: true},
			{name: "explicit override", env: "synthetic-secret", args: []string{"--token=explicit"}, want: "explicit", login: true},
			{name: "explicit empty errors", env: "synthetic-secret", args: []string{"--token="}, login: true},
			{name: "empty environment skips login"},
		} {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				t.Setenv("GPUD_TOKEN", tt.env)
				dir := t.TempDir()
				mockey.PatchConvey("consume token before startup", t, func() {
					mockey.Mock(common.ResolveDataDir).To(func(*cli.Context) (string, error) {
						_, present := os.LookupEnv("GPUD_TOKEN")
						require.False(t, present, "clear credentials before any startup work")
						return dir, nil
					}).Build()
					mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
					loginCalled := false
					stopLogin := errors.New("stop after login")
					mockey.Mock(login.Login).To(func(_ context.Context, cfg login.LoginConfig) error {
						loginCalled = true
						require.Equal(t, tt.want, cfg.Token)
						if cfg.Token == "" {
							return login.ErrEmptyToken
						}
						return stopLogin
					}).Build()
					stopStartup := errors.New("stop before systemd startup")
					mockey.Mock(os.Executable).To(func() (string, error) { return "", stopStartup }).Build()
					args := append([]string{"gpud", name, "--log-level=info"}, tt.args...)
					if name == "run" {
						// Invalid post-login configuration prevents starting a real daemon.
						args = append(args, "--infiniband-expected-port-states=invalid")
					}
					err := App().Run(args)
					require.Equal(t, tt.login, loginCalled)
					switch {
					case tt.login && tt.want == "":
						require.ErrorIs(t, err, login.ErrEmptyToken)
					case tt.login:
						require.ErrorIs(t, err, stopLogin)
					case name == "up":
						require.ErrorIs(t, err, stopStartup)
					default:
						require.ErrorContains(t, err, "invalid character")
					}
				})
			})
		}
	}
}
