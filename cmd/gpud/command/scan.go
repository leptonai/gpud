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
	err := diagnose.Scan(ctx, tailLines, debug)
	if err != nil {
		return err
	}

	return nil
}
