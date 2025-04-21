package systemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDefaultEnvFileContent(t *testing.T) {
	t.Run("without endpoint", func(t *testing.T) {
		content := createDefaultEnvFileContent("")
		assert.Equal(t, defaultEnvFileContent, content)
		assert.Contains(t, content, "--log-level=info --log-file=/var/log/gpud.log")
		assert.NotContains(t, content, "--endpoint=")
	})

	t.Run("with endpoint", func(t *testing.T) {
		endpoint := "https://example.com"
		content := createDefaultEnvFileContent(endpoint)
		assert.Contains(t, content, "--log-level=info --log-file=/var/log/gpud.log")
		assert.Contains(t, content, "--endpoint=https://example.com")
	})
}

func TestProcessEnvFileLines(t *testing.T) {
	t.Run("file without FLAGS", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file without FLAGS line
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := "# gpud environment variables\nSOME_OTHER_VAR=\"value\""
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function
		lines, err := processEnvFileLines(testFile, "")
		require.NoError(t, err)

		// Verify results
		assert.Len(t, lines, 2)
		assert.Equal(t, "# gpud environment variables", lines[0])
		assert.Equal(t, "SOME_OTHER_VAR=\"value\"", lines[1])
	})

	t.Run("file with FLAGS missing log-file", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS but without log file flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		err = os.WriteFile(testFile, []byte("FLAGS=\"--log-level=debug\""), 0644)
		require.NoError(t, err)

		// Call the function with empty endpoint
		lines, err := processEnvFileLines(testFile, "")
		require.NoError(t, err)

		// Verify results
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0], "--log-level=debug --log-file=/var/log/gpud.log")
		assert.NotContains(t, lines[0], "--endpoint=")
	})

	t.Run("file with FLAGS missing endpoint", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS but without endpoint flag
		testFile := filepath.Join(tmpDir, "gpud-env")
		err = os.WriteFile(testFile, []byte("FLAGS=\"--log-level=debug --log-file=/var/log/gpud.log\""), 0644)
		require.NoError(t, err)

		// Call the function with endpoint
		endpoint := "https://example.com"
		lines, err := processEnvFileLines(testFile, endpoint)
		require.NoError(t, err)

		// Verify results
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0], "--log-level=debug --log-file=/var/log/gpud.log")
		assert.Contains(t, lines[0], "--endpoint=https://example.com")
	})

	t.Run("file with FLAGS having both log-file and endpoint", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS containing both flags
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := "FLAGS=\"--log-level=debug --log-file=/var/log/gpud.log --endpoint=https://example.com\""
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function with the same endpoint
		endpoint := "https://example.com"
		lines, err := processEnvFileLines(testFile, endpoint)
		require.NoError(t, err)

		// Verify results
		assert.Len(t, lines, 1)
		assert.Equal(t, originalContent, lines[0])
	})

	t.Run("file with invalid format", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file that doesn't exist
		testFile := filepath.Join(tmpDir, "non-existent-file")

		// Call the function with invalid file
		_, err = processEnvFileLines(testFile, "")
		assert.Error(t, err)
	})
}

func TestWriteEnvFile(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file path that doesn't exist yet
		testFile := filepath.Join(tmpDir, "gpud-env")

		// Call the function to create the file
		err = writeEnvFile(testFile, "")
		require.NoError(t, err)

		// Check if the file was created with the correct content
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--log-level=info --log-file=/var/log/gpud.log")
	})

	t.Run("file does not exist with endpoint", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file path that doesn't exist yet
		testFile := filepath.Join(tmpDir, "gpud-env")
		endpoint := "https://example.com"

		// Call the function to create the file
		err = writeEnvFile(testFile, endpoint)
		require.NoError(t, err)

		// Check if the file was created with the correct content
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--log-level=info --log-file=/var/log/gpud.log")
		assert.Contains(t, string(content), "--endpoint=https://example.com")
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
		err = writeEnvFile(testFile, "")
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
		content := "# gpud environment variables\nFLAGS=\"--log-level=debug --log-file=/custom/path.log\""
		err = os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = writeEnvFile(testFile, "")
		require.NoError(t, err)

		// Check if the file was not modified (we check for the important parts)
		updatedContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		updatedContentStr := string(updatedContent)

		assert.Contains(t, updatedContentStr, "--log-level=debug")
		assert.Contains(t, updatedContentStr, "--log-file=/custom/path.log")
		assert.NotContains(t, updatedContentStr, "--endpoint=")
	})
}

func TestUpdateFlagsFromExistingEnvFile(t *testing.T) {
	t.Run("file exists without FLAGS line", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file without FLAGS line
		testFile := filepath.Join(tmpDir, "gpud-env")
		originalContent := `# gpud environment variables
SOME_OTHER_VAR="value"`
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = updateFlagsFromExistingEnvFile(testFile, "")
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
		err = updateFlagsFromExistingEnvFile(testFile, "")
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
		content := "# gpud environment variables\nFLAGS=\"--log-level=debug --log-file=/custom/path.log\""
		err = os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = updateFlagsFromExistingEnvFile(testFile, "")
		require.NoError(t, err)

		// Check if the file was not modified (we check for the important parts)
		updatedContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		updatedContentStr := string(updatedContent)

		assert.Contains(t, updatedContentStr, "--log-level=debug")
		assert.Contains(t, updatedContentStr, "--log-file=/custom/path.log")
		assert.NotContains(t, updatedContentStr, "--endpoint=")
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
FLAGS="--other-flag=true"`
		err = os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Call the function to update the file
		err = updateFlagsFromExistingEnvFile(testFile, "")
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

	t.Run("update with endpoint parameter", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a file with FLAGS but without endpoint
		testFile := filepath.Join(tmpDir, "gpud-env")
		err = os.WriteFile(testFile, []byte(`FLAGS="--log-level=debug --log-file=/var/log/gpud.log"`), 0644)
		require.NoError(t, err)

		// Call the function with endpoint
		endpoint := "https://example.com"
		err = updateFlagsFromExistingEnvFile(testFile, endpoint)
		require.NoError(t, err)

		// Check if the endpoint was added
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "--endpoint=https://example.com")
	})

	t.Run("error handling with invalid file", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Path to a file that doesn't exist
		nonExistentFile := filepath.Join(tmpDir, "does-not-exist")

		// Call the function with a file that doesn't exist
		err = updateFlagsFromExistingEnvFile(nonExistentFile, "")
		assert.Error(t, err, "Should return an error when the file doesn't exist")
	})
}
