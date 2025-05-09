// Package root implements the root command for the "gpud" command.
package root

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gpud",
	Short: "GPUd tool",
	Example: `
# to quick scan for your machine health status
gpud scan

# to start gpud as a systemd unit
sudo gpud up
`,
}

// Command returns the root command for the "gpud" command.
func Command() *cobra.Command {
	return rootCmd
}

// AddCommand adds a subcommand to the root command.
func AddCommand(subCmd *cobra.Command) {
	rootCmd.AddCommand(
		subCmd,
	)
}

var (
	logLevel string
	logFile  string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "set the logging level [debug, info, warn, error, fatal, panic, dpanic]")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "set the log file path (set empty to stdout/stderr)")
}

// FlagLogLevel returns the log level flag value.
func FlagLogLevel(cmd *cobra.Command) (string, error) {
	return cmd.PersistentFlags().GetString("log-level")
}

// FlagLogFile returns the log file flag value.
func FlagLogFile(cmd *cobra.Command) (string, error) {
	return cmd.PersistentFlags().GetString("log-file")
}
