package command

import (
	"context"
	"fmt"
	"time"

	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"

	"github.com/urfave/cli"
)

func cmdIsNvidia(cliContext *cli.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nvidiaInstalled, err := nvidiaquery.GPUsInstalled(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("NVIDIA installed: %v", nvidiaInstalled)
	return nil
}
