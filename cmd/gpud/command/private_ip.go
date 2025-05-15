package command

import (
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

func cmdPrivateIP(cliContext *cli.Context) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

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
