package command

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components/diagnose"

	"github.com/urfave/cli"
)

func cmdScan(cliContext *cli.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := diagnose.Scan(
		ctx,
		diagnose.WithLines(tailLines),
		diagnose.WithDebug(debug),
		diagnose.WithPollXidEvents(pollXidEvents),
		diagnose.WithPollGPMEvents(pollGPMEvents),
		diagnose.WithNetcheck(netcheck),
	)
	if err != nil {
		return err
	}

	return nil
}
