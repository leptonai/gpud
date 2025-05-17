package privateip

import (
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	ips, err := netutil.GetPrivateIPs(
		netutil.WithPrefixesToSkip(
			"lo",
			"eni",
			"cali",
			"docker",
			"lepton",
			"tailscale",
		),
		netutil.WithSuffixesToSkip(".calico"),
	)
	if err != nil {
		return err
	}

	ips.RenderTable(os.Stdout)

	return nil
}
