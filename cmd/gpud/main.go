package main

import (
	"fmt"
	"os"

	"github.com/leptonai/gpud/cmd/gpud/command"
)

func main() {
	app := command.App()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "gpud: %s\n", err)
		os.Exit(1)
	}
}
