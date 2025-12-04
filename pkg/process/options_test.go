package process

import (
	"os"
	"testing"
	"time"
)

// TestOpApplyOpts tests the applyOpts function of Op
func TestOpApplyOpts(t *testing.T) {
	// Test with no options
	op := &Op{}
	err := op.applyOpts([]OpOption{})
	if err == nil {
		t.Fatal("Expected error for no command, but got nil")
	}
	if op.labels == nil {
		t.Fatal("Expected labels to be initialized, but it's nil")
	}

	// Test with command
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(op.commandsToRun) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(op.commandsToRun))
	}
	if op.commandsToRun[0][0] != "echo" || op.commandsToRun[0][1] != "hello" {
		t.Fatalf("Expected command 'echo hello', got %v", op.commandsToRun[0])
	}

	// Test with bash script contents
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithBashScriptContentsToRun("echo hello"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if op.bashScriptContentsToRun != "echo hello" {
		t.Fatalf("Expected bash script contents 'echo hello', got %q", op.bashScriptContentsToRun)
	}
	if !op.runAsBashScript {
		t.Fatal("Expected runAsBashScript to be true, but it's false")
	}

	// Test with multiple commands without bash script mode
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithCommand("echo", "world"),
	})
	if err == nil {
		t.Fatal("Expected error for multiple commands without bash script mode, but got nil")
	}

	// Test with multiple commands with bash script mode
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithCommand("echo", "world"),
		WithRunAsBashScript(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(op.commandsToRun) != 2 {
		t.Fatalf("Expected 2 commands, got %d", len(op.commandsToRun))
	}
	if !op.runAsBashScript {
		t.Fatal("Expected runAsBashScript to be true, but it's false")
	}

	// Test with invalid command
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("non_existent_command_12345"),
	})
	if err == nil {
		t.Fatal("Expected error for invalid command, but got nil")
	}

	// Test with environment variables
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("VAR1=value1", "VAR2=value2"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(op.envs) != 2 {
		t.Fatalf("Expected 2 environment variables, got %d", len(op.envs))
	}
	if op.envs[0] != "VAR1=value1" || op.envs[1] != "VAR2=value2" {
		t.Fatalf("Expected environment variables 'VAR1=value1' and 'VAR2=value2', got %v", op.envs)
	}

	// Test with invalid environment variable format
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("INVALID_ENV_VAR"),
	})
	if err == nil {
		t.Fatal("Expected error for invalid environment variable format, but got nil")
	}

	// Test with duplicate environment variables
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithEnvs("VAR=value1", "VAR=value2"),
	})
	if err == nil {
		t.Fatal("Expected error for duplicate environment variables, but got nil")
	}

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
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if op.restartConfig.Interval != 5*time.Second {
		t.Fatalf("Expected interval to be 5s, got %s", op.restartConfig.Interval)
	}

	// Test with custom bash script directory and pattern
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithRunAsBashScript(),
		WithBashScriptTmpDirectory("/tmp"),
		WithBashScriptFilePattern("custom-*.sh"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if op.bashScriptTmpDirectory != "/tmp" {
		t.Fatalf("Expected bash script directory '/tmp', got %q", op.bashScriptTmpDirectory)
	}
	if op.bashScriptFilePattern != "custom-*.sh" {
		t.Fatalf("Expected bash script pattern 'custom-*.sh', got %q", op.bashScriptFilePattern)
	}

	// Test with default bash script directory and pattern
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithRunAsBashScript(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if op.bashScriptTmpDirectory != os.TempDir() {
		t.Fatalf("Expected bash script directory %q, got %q", os.TempDir(), op.bashScriptTmpDirectory)
	}
	if op.bashScriptFilePattern != DefaultBashScriptFilePattern {
		t.Fatalf("Expected bash script pattern %q, got %q", DefaultBashScriptFilePattern, op.bashScriptFilePattern)
	}

	// Test with output file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if op.outputFile != tmpFile {
		t.Fatal("Expected output file to be set, but it's not")
	}

	// Test with labels
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommand("echo", "hello"),
		WithLabel("key1", "value1"),
		WithLabel("key2", "value2"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(op.labels) != 2 {
		t.Fatalf("Expected 2 labels, got %d", len(op.labels))
	}
	if op.labels["key1"] != "value1" || op.labels["key2"] != "value2" {
		t.Fatalf("Expected labels 'key1=value1' and 'key2=value2', got %v", op.labels)
	}

	// Test with commands
	op = &Op{}
	err = op.applyOpts([]OpOption{
		WithCommands([][]string{
			{"echo", "hello"},
			{"echo", "world"},
		}),
		WithRunAsBashScript(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(op.commandsToRun) != 2 {
		t.Fatalf("Expected 2 commands, got %d", len(op.commandsToRun))
	}
	if op.commandsToRun[0][0] != "echo" || op.commandsToRun[0][1] != "hello" {
		t.Fatalf("Expected command 'echo hello', got %v", op.commandsToRun[0])
	}
	if op.commandsToRun[1][0] != "echo" || op.commandsToRun[1][1] != "world" {
		t.Fatalf("Expected command 'echo world', got %v", op.commandsToRun[1])
	}
}

// TestCommandExists tests the commandExists function
func TestCommandExists(t *testing.T) {
	// Test with existing command
	if !commandExists("echo") {
		t.Fatal("Expected 'echo' command to exist, but it doesn't")
	}

	// Test with non-existent command
	if commandExists("non_existent_command_12345") {
		t.Fatal("Expected 'non_existent_command_12345' command to not exist, but it does")
	}
}
