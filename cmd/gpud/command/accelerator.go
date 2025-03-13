package command

import (
	"fmt"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/accelerator"
)

func cmdAccelerator(cliContext *cli.Context) error {
	acceleratorType, productName, err := accelerator.DetectTypeAndProductName()
	if err != nil {
		return err
	}

	fmt.Printf("accelerator type: %s\nproduct name: %s\n", acceleratorType, productName)
	return nil
}
