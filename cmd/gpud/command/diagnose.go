package command

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/leptonai/gpud/internal/diagnose"

	"github.com/urfave/cli"
)

func cmdDiagnose(cliContext *cli.Context) error {
	if os.Geteuid() != 0 {
		return errors.New("requires sudo/root access to diagnose GPU issues")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := diagnose.Run(ctx, diagnose.WithCreateArchive(createArchive))
	if err != nil {
		return err
	}

	return nil
}
