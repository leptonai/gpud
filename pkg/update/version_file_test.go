package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/version"
)

func TestCheckVersionFileForUpdate(t *testing.T) {
	// Save original version for restoration
	originalVersion := version.Version
	defer func() {
		version.Version = originalVersion
	}()

	tests := []struct {
		name           string
		versionFile    string
		fileContent    string
		currentVersion string
		createFile     bool
		expectVersion  string
		expectUpdate   bool
		expectError    bool
	}{
		{
			name:           "empty version file path",
			versionFile:    "",
			currentVersion: "1.0.0",
			createFile:     false,
			expectVersion:  "",
			expectUpdate:   false,
			expectError:    false,
		},
		{
			name:           "file does not exist",
			versionFile:    "nonexistent.txt",
			currentVersion: "1.0.0",
			createFile:     false,
			expectVersion:  "",
			expectUpdate:   false,
			expectError:    false,
		},
		{
			name:           "same version",
			versionFile:    "version.txt",
			fileContent:    "1.0.0",
			currentVersion: "1.0.0",
			createFile:     true,
			expectVersion:  "1.0.0",
			expectUpdate:   false,
			expectError:    false,
		},
		{
			name:           "different version",
			versionFile:    "version.txt",
			fileContent:    "2.0.0",
			currentVersion: "1.0.0",
			createFile:     true,
			expectVersion:  "2.0.0",
			expectUpdate:   true,
			expectError:    false,
		},
		{
			name:           "version with whitespace",
			versionFile:    "version.txt",
			fileContent:    "  2.0.0\n\t",
			currentVersion: "1.0.0",
			createFile:     true,
			expectVersion:  "2.0.0",
			expectUpdate:   true,
			expectError:    false,
		},
		{
			name:           "empty file",
			versionFile:    "version.txt",
			fileContent:    "",
			currentVersion: "1.0.0",
			createFile:     true,
			expectVersion:  "",
			expectUpdate:   true,
			expectError:    false,
		},
		{
			name:           "whitespace only file",
			versionFile:    "version.txt",
			fileContent:    "   \n\t  ",
			currentVersion: "1.0.0",
			createFile:     true,
			expectVersion:  "",
			expectUpdate:   true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set current version for test
			version.Version = tt.currentVersion

			tempDir := t.TempDir()
			versionFilePath := tt.versionFile
			if tt.versionFile != "" && tt.versionFile != "nonexistent.txt" {
				versionFilePath = filepath.Join(tempDir, tt.versionFile)
			}

			if tt.createFile && tt.versionFile != "" {
				err := os.WriteFile(versionFilePath, []byte(tt.fileContent), 0644)
				require.NoError(t, err)
			}

			targetVer, needUpdate, err := checkVersionFileForUpdate(versionFilePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectVersion, targetVer)
				assert.Equal(t, tt.expectUpdate, needUpdate)
			}
		})
	}
}

func TestCheckVersionFileForUpdate_PermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Cannot test permission errors when running as root")
	}

	tempDir := t.TempDir()
	versionFile := filepath.Join(tempDir, "version.txt")

	// Create file with no read permissions
	err := os.WriteFile(versionFile, []byte("1.0.0"), 0000)
	require.NoError(t, err)

	_, _, err = checkVersionFileForUpdate(versionFile)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, os.ErrNotExist))
}

func TestUpdateTargetVersion(t *testing.T) {
	// Save original version for restoration
	originalVersion := version.Version
	defer func() {
		version.Version = originalVersion
	}()

	tests := []struct {
		name             string
		versionFile      string
		fileContent      string
		currentVersion   string
		autoExitCode     int
		systemdManaged   bool
		updateError      error
		expectUpdateCall bool
		expectExitCall   bool
		expectedExitCode int
		expectError      bool
	}{
		{
			name:             "no update needed - same version",
			versionFile:      "version.txt",
			fileContent:      "1.0.0",
			currentVersion:   "1.0.0",
			autoExitCode:     0,
			systemdManaged:   false,
			expectUpdateCall: false,
			expectExitCall:   false,
			expectError:      false,
		},
		{
			name:             "update needed - systemd managed",
			versionFile:      "version.txt",
			fileContent:      "2.0.0",
			currentVersion:   "1.0.0",
			autoExitCode:     0,
			systemdManaged:   true,
			expectUpdateCall: true,
			expectExitCall:   true,
			expectedExitCode: 0,
			expectError:      false,
		},
		{
			name:             "update needed - not systemd managed",
			versionFile:      "version.txt",
			fileContent:      "2.0.0",
			currentVersion:   "1.0.0",
			autoExitCode:     1,
			systemdManaged:   false,
			expectUpdateCall: true,
			expectExitCall:   true,
			expectedExitCode: 1,
			expectError:      false,
		},
		{
			name:             "update needed - no auto exit",
			versionFile:      "version.txt",
			fileContent:      "2.0.0",
			currentVersion:   "1.0.0",
			autoExitCode:     -1,
			systemdManaged:   false,
			expectUpdateCall: true,
			expectExitCall:   false,
			expectError:      false,
		},
		{
			name:             "update fails",
			versionFile:      "version.txt",
			fileContent:      "2.0.0",
			currentVersion:   "1.0.0",
			autoExitCode:     0,
			systemdManaged:   false,
			updateError:      errors.New("update failed"),
			expectUpdateCall: true,
			expectExitCall:   false,
			expectError:      true,
		},
		{
			name:             "empty version file path",
			versionFile:      "",
			currentVersion:   "1.0.0",
			autoExitCode:     0,
			systemdManaged:   false,
			expectUpdateCall: false,
			expectExitCall:   false,
			expectError:      false,
		},
		{
			name:             "file does not exist",
			versionFile:      "nonexistent.txt",
			currentVersion:   "1.0.0",
			autoExitCode:     0,
			systemdManaged:   false,
			expectUpdateCall: false,
			expectExitCall:   false,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set current version for test
			version.Version = tt.currentVersion

			tempDir := t.TempDir()
			versionFilePath := tt.versionFile
			if tt.versionFile != "" && tt.versionFile != "nonexistent.txt" {
				versionFilePath = filepath.Join(tempDir, tt.versionFile)
				if tt.fileContent != "" {
					err := os.WriteFile(versionFilePath, []byte(tt.fileContent), 0644)
					require.NoError(t, err)
				}
			}

			// Track function calls
			var updateCalled bool
			var capturedTargetVersion string
			var capturedURL string
			var capturedRequireRoot bool
			var exitCalled bool
			var exitCode int

			checkGPUdSystemdServiceFunc := func() bool {
				return tt.systemdManaged
			}

			updateExecutableFunc := func(targetVersion string, url string, requireRoot bool) error {
				updateCalled = true
				capturedTargetVersion = targetVersion
				capturedURL = url
				capturedRequireRoot = requireRoot
				return tt.updateError
			}

			osExitFunc := func(code int) {
				exitCalled = true
				exitCode = code
			}

			err := updateTargetVersion(
				versionFilePath,
				tt.autoExitCode,
				checkGPUdSystemdServiceFunc,
				updateExecutableFunc,
				osExitFunc,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectUpdateCall, updateCalled, "update function call mismatch")
			if tt.expectUpdateCall {
				assert.Equal(t, tt.fileContent, capturedTargetVersion, "target version mismatch")
				assert.Equal(t, DefaultUpdateURL, capturedURL, "update URL mismatch")
				assert.Equal(t, tt.systemdManaged, capturedRequireRoot, "require root mismatch")
			}

			assert.Equal(t, tt.expectExitCall, exitCalled, "exit function call mismatch")
			if tt.expectExitCall {
				assert.Equal(t, tt.expectedExitCode, exitCode, "exit code mismatch")
			}
		})
	}
}

func TestUpdateTargetVersion_ReadError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Cannot test permission errors when running as root")
	}

	tempDir := t.TempDir()
	versionFile := filepath.Join(tempDir, "version.txt")

	// Create file with no read permissions
	err := os.WriteFile(versionFile, []byte("2.0.0"), 0000)
	require.NoError(t, err)

	checkGPUdSystemdServiceFunc := func() bool {
		return false
	}

	updateExecutableFunc := func(targetVersion string, url string, requireRoot bool) error {
		t.Fatal("update function should not be called")
		return nil
	}

	osExitFunc := func(code int) {
		t.Fatal("exit function should not be called")
	}

	err = updateTargetVersion(
		versionFile,
		0,
		checkGPUdSystemdServiceFunc,
		updateExecutableFunc,
		osExitFunc,
	)

	assert.Error(t, err)
	assert.False(t, errors.Is(err, os.ErrNotExist))
}

// Benchmark tests
func BenchmarkCheckVersionFileForUpdate_FileExists(b *testing.B) {
	tempDir := b.TempDir()
	versionFile := filepath.Join(tempDir, "version.txt")
	err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
	require.NoError(b, err)

	version.Version = "1.0.0"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = checkVersionFileForUpdate(versionFile)
	}
}

func BenchmarkCheckVersionFileForUpdate_FileNotExists(b *testing.B) {
	tempDir := b.TempDir()
	versionFile := filepath.Join(tempDir, "nonexistent.txt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = checkVersionFileForUpdate(versionFile)
	}
}
