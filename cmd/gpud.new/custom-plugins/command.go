// Package customplugins implements the "custom-plugins" command.
package customplugins

import (
	"fmt"

	"github.com/leptonai/gpud/cmd/gpud.new/root"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "custom-plugins",
	Aliases: []string{"cs"},
	Short:   "Custom plugins",
	Long:    `Custom plugins are used to monitor custom metrics.`,
	RunE:    run,
}

func Command() *cobra.Command {
	return rootCmd
}

func run(cmd *cobra.Command, args []string) error {
	logLevel, err := root.FlagLogLevel(cmd)
	if err != nil {
		return err
	}
	logFile, err := root.FlagLogFile(cmd)
	if err != nil {
		return err
	}
	fmt.Println(logLevel, logFile)

	return nil
}
