// "gpud" implements the "gpud" command-line interface.
package main

import (
	"fmt"
	"os"

	cmdcustomplugins "github.com/leptonai/gpud/cmd/gpud.new/custom-plugins"
	"github.com/leptonai/gpud/cmd/gpud.new/root"
)

func main() {
	root.AddCommand(cmdcustomplugins.Command())

	if err := root.Command().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "'gpud' failed %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
