package root

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommand(t *testing.T) {
	cmd := Command()
	assert.Equal(t, "gpud", cmd.Use)
	assert.Equal(t, "GPUd tool", cmd.Short)
	assert.True(t, strings.Contains(cmd.Example, "gpud scan"))
	assert.True(t, strings.Contains(cmd.Example, "sudo gpud up"))
}

func TestAddCommand(t *testing.T) {
	// Save the original commands to restore after test
	originalCommands := rootCmd.Commands()
	defer func() {
		// Reset the rootCmd commands
		rootCmd.ResetCommands()
		for _, cmd := range originalCommands {
			rootCmd.AddCommand(cmd)
		}
	}()

	// Create a test subcommand
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Get initial number of subcommands
	initialCmdCount := len(rootCmd.Commands())

	// Add the test command
	AddCommand(testCmd)

	// Verify the command was added
	assert.Equal(t, initialCmdCount+1, len(rootCmd.Commands()))

	// Find the added command
	var found bool
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "test" {
			found = true
			break
		}
	}
	assert.True(t, found, "Added command was not found")
}

func TestFlags(t *testing.T) {
	cmd := Command()

	// Test log-level flag
	logLevelFlag := cmd.PersistentFlags().Lookup("log-level")
	assert.NotNil(t, logLevelFlag)
	assert.Equal(t, "info", logLevelFlag.DefValue)

	// Test log-file flag
	logFileFlag := cmd.PersistentFlags().Lookup("log-file")
	assert.NotNil(t, logFileFlag)
	assert.Equal(t, "", logFileFlag.DefValue)
}

func TestFlagLogLevel(t *testing.T) {
	cmd := Command()

	// Default value
	level, err := FlagLogLevel(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "info", level)

	// Changed value
	err = cmd.PersistentFlags().Set("log-level", "debug")
	assert.NoError(t, err)

	level, err = FlagLogLevel(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "debug", level)
}

func TestFlagLogFile(t *testing.T) {
	cmd := Command()

	// Default value
	file, err := FlagLogFile(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "", file)

	// Changed value
	err = cmd.PersistentFlags().Set("log-file", "/tmp/logfile.log")
	assert.NoError(t, err)

	file, err = FlagLogFile(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/logfile.log", file)
}
