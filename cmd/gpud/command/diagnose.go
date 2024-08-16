package command

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/leptonai/gpud/components/diagnose"

	"github.com/urfave/cli"
)

func cmdDiagnose(cliContext *cli.Context) error {
	if os.Geteuid() != 0 {
		return errors.New("diagnose requires root")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := diagnose.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}
