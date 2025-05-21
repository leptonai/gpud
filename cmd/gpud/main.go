package main

import (
	"fmt"
	"os"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/cmd/gpud/command"
)

func main() {
	app := command.App()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}
}
