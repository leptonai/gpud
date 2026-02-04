package process

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOpApplyOpts tests the applyOpts function of Op
func TestOpApplyOpts(t *testing.T) {
	// Test with no options
	op := &Op{}
	err := op.applyOpts([]OpOption{})
	require.Error(t, err, "Expected error for no command, but got nil")
	require.NotNil(t, op.labels, "Expected labels to be initialized, but it's nil")

	// Test with command
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
	})
	require.NoError(t, err, "Expected no error")
	require.Len(t, op.commandsToRun, 1)
	require.Equal(t, []string{"echo", "hello"}, op.commandsToRun[0])

	// Test with bash script contents
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithBashScriptContentsToRun("echo hello"),
	})
	require.NoError(t, err, "Expected no error")
	require.Equal(t, "echo hello", op.bashScriptContentsToRun)
	require.True(t, op.runAsBashScript, "Expected runAsBashScript to be true, but it's false")

	// Test with multiple commands without bash script mode
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithCommand("echo", "world"),
	})
	require.Error(t, err, "Expected error for multiple commands without bash script mode, but got nil")

	// Test with multiple commands with bash script mode
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithCommand("echo", "world"),
		WithRunAsBashScript(),
	})
	require.NoError(t, err, "Expected no error")
	require.Len(t, op.commandsToRun, 2)
	require.True(t, op.runAsBashScript, "Expected runAsBashScript to be true, but it's false")

	// Test with invalid command
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("non_existent_command_12345"),
	})
	require.Error(t, err, "Expected error for invalid command, but got nil")

	// Test with environment variables
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("VAR1=value1", "VAR2=value2"),
	})
	require.NoError(t, err, "Expected no error")
	require.Len(t, op.envs, 2)
	require.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, op.envs)

	// Test with invalid environment variable format
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("INVALID_ENV_VAR"),
	})
	require.Error(t, err, "Expected error for invalid environment variable format, but got nil")

	// Test with duplicate environment variables
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("VAR=value1", "VAR=value2"),
	})
	require.Error(t, err, "Expected error for duplicate environment variables, but got nil")

	// Test with restart config with zero interval
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithRestartConfig(RestartConfig{
			OnError:  true,
			Limit:    1,
			Interval: 0,
		}),
	})
	require.NoError(t, err, "Expected no error")
	require.Equal(t, 5*time.Second, op.restartConfig.Interval)

	// Test with custom bash script directory and pattern
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithRunAsBashScript(),
		WithBashScriptTmpDirectory("/tmp"),
		WithBashScriptFilePattern("custom-*.sh"),
	})
	require.NoError(t, err, "Expected no error")
	require.Equal(t, "/tmp", op.bashScriptTmpDirectory)
	require.Equal(t, "custom-*.sh", op.bashScriptFilePattern)

	// Test with default bash script directory and pattern
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithRunAsBashScript(),
	})
	require.NoError(t, err, "Expected no error")
	require.Equal(t, os.TempDir(), op.bashScriptTmpDirectory)
	require.Equal(t, DefaultBashScriptFilePattern, op.bashScriptFilePattern)

	// Test with output file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	defer func() {
		_ = tmpFile.Close()
	}()

	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithOutputFile(tmpFile),
	})
	require.NoError(t, err, "Expected no error")
	require.Same(t, tmpFile, op.outputFile, "Expected output file to be set")

	// Test with labels
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithLabel("key1", "value1"),
		WithLabel("key2", "value2"),
	})
	require.NoError(t, err, "Expected no error")
	require.Len(t, op.labels, 2)
	require.Equal(t, "value1", op.labels["key1"])
	require.Equal(t, "value2", op.labels["key2"])

	// Test with commands
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommands([][]string{
			{"echo", "hello"},
			{"echo", "world"},
		}),
		WithRunAsBashScript(),
	})
	require.NoError(t, err, "Expected no error")
	require.Len(t, op.commandsToRun, 2)
	require.Equal(t, []string{"echo", "hello"}, op.commandsToRun[0])
	require.Equal(t, []string{"echo", "world"}, op.commandsToRun[1])
}

// TestCommandExists tests the commandExists function
func TestCommandExists(t *testing.T) {
	// Test with existing command
	require.True(t, commandExists("echo"), "Expected 'echo' command to exist, but it doesn't")

	// Test with non-existent command
	require.False(t, commandExists("non_existent_command_12345"), "Expected 'non_existent_command_12345' command to not exist, but it does")
}
