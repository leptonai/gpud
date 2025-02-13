package command

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/accelerator"

	"github.com/urfave/cli"
)

func cmdAccelerator(cliContext *cli.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	acceleratorType, productName, err := accelerator.DetectTypeAndProductName(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("accelerator type: %s\nproduct name: %s\n", acceleratorType, productName)
	return nil
}
