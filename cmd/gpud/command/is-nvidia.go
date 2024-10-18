package command

import (
	"context"
	"fmt"
	"time"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"

	"github.com/urfave/cli"
)

func cmdIsNvidia(cliContext *cli.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("NVIDIA installed: %v", nvidiaInstalled)
	return nil
}
