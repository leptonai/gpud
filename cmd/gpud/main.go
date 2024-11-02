package main

import (
	"fmt"
	"os"

	"github.com/leptonai/gpud/cmd/gpud/command"
	"github.com/leptonai/gpud/version"
)

func main() {
	// check for version flag before initializing the full app
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("gpud version", version.Version)
		return
	}

	app := command.App()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "gpud: %s\n", err)
		os.Exit(1)
	}
}
