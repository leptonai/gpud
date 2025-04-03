package systemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteEnvFile(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file path that doesn't exist yet
		testFile := filepath.Join(tmpDir, "gpud-env")

		// Call the function to create the file
		err = writeEnvFile(testFile)
		require.NoError(t, err)

		// Check if the file was created with the correct content
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--log-level=info --log-file=/var/log/gpud.log")
	})

	t.Run("file exists without log file flag", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with existing flags but without log file flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		err = os.WriteFile(testFile, []byte(`# gpud environment variables
FLAGS="--log-level=debug"
`), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = writeEnvFile(testFile)
		require.NoError(t, err)

		// Check if the file was updated with the log file flag
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--log-level=debug --log-file=/var/log/gpud.log")
	})

	t.Run("file exists with log file flag", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with existing flags including log file flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		existingContent := `# gpud environment variables
FLAGS="--log-level=debug --log-file=/custom/path.log"
`
		err = os.WriteFile(testFile, []byte(existingContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = writeEnvFile(testFile)
		require.NoError(t, err)

		// Check if the file was not modified
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content))
	})
}

func TestAddLogFileFlagIfExists(t *testing.T) {
	t.Run("file exists without FLAGS line", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file without FLAGS line
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := `# gpud environment variables
SOME_OTHER_VAR="value"
`
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = addLogFileFlagIfExists(testFile)
		require.NoError(t, err)

		// Check if the file content was not modified
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(content))
	})

	t.Run("file exists with FLAGS but without log file flag", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS but without log file flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		err = os.WriteFile(testFile, []byte(`# gpud environment variables
FLAGS="--log-level=debug"
`), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = addLogFileFlagIfExists(testFile)
		require.NoError(t, err)

		// Check if the file was updated with the log file flag
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--log-level=debug --log-file=/var/log/gpud.log")
	})

	t.Run("file exists with FLAGS already having log file flag", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS already containing log file flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := `# gpud environment variables
FLAGS="--log-level=debug --log-file=/custom/path.log"
`
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = addLogFileFlagIfExists(testFile)
		require.NoError(t, err)

		// Check if the file was not modified
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(content))
	})

	t.Run("file exists with multiple FLAGS lines", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with multiple FLAGS lines
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := `# gpud environment variables
FLAGS="--log-level=debug"
# Another section
FLAGS="--other-flag=true"
`
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = addLogFileFlagIfExists(testFile)
		require.NoError(t, err)

		// Check if all FLAGS lines were updated
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		lines := strings.Split(string(content), "\n")

		flagsCount := 0
		for _, line := range lines {
			if strings.Contains(line, "FLAGS=") {
				assert.Contains(t, line, "--log-file=/var/log/gpud.log")
				flagsCount++
			}
		}
		assert.Equal(t, 2, flagsCount, "Both FLAGS lines should be updated")
	})
}
