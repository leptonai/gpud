package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"
)

// TestUpdateTargetVersion_WithRealSystemdCheck tests UpdateTargetVersion with mocked systemd check.
func TestUpdateTargetVersion_WithRealSystemdCheck(t *testing.T) {
	mockey.PatchConvey("with real systemd check", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
		require.NoError(t, err)

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return true, nil
		}).Build()

		updateCalled := false
		var capturedRequireRoot bool
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			capturedRequireRoot = requireRoot
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err = UpdateTargetVersion(versionFile, 0)
		require.NoError(t, err)
		assert.True(t, updateCalled)
		assert.True(t, capturedRequireRoot, "requireRoot should be true when systemd managed")
		assert.True(t, exitCalled)
	})
}

// TestUpdateTargetVersion_SystemdNotActive tests UpdateTargetVersion when systemd is not active.
func TestUpdateTargetVersion_SystemdNotActive(t *testing.T) {
	mockey.PatchConvey("systemd not active", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
		require.NoError(t, err)

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		updateCalled := false
		var capturedRequireRoot bool
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			capturedRequireRoot = requireRoot
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err = UpdateTargetVersion(versionFile, 0)
		require.NoError(t, err)
		assert.True(t, updateCalled)
		assert.False(t, capturedRequireRoot, "requireRoot should be false when not systemd managed")
		assert.True(t, exitCalled)
	})
}

// TestUpdateTargetVersion_SystemdCheckError tests UpdateTargetVersion when systemd check returns error.
func TestUpdateTargetVersion_SystemdCheckError(t *testing.T) {
	mockey.PatchConvey("systemd check error", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
		require.NoError(t, err)

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, errors.New("systemd error")
		}).Build()

		updateCalled := false
		var capturedRequireRoot bool
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			capturedRequireRoot = requireRoot
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		// Even on error, the function should proceed with requireRoot = false
		err = UpdateTargetVersion(versionFile, 0)
		require.NoError(t, err)
		assert.True(t, updateCalled)
		assert.False(t, capturedRequireRoot, "requireRoot should be false on systemd check error")
		assert.True(t, exitCalled)
	})
}

// TestUpdateTargetVersion_NoUpdateNeeded tests UpdateTargetVersion when no update is needed.
func TestUpdateTargetVersion_NoUpdateNeeded(t *testing.T) {
	mockey.PatchConvey("no update needed", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("1.0.0"), 0644)
		require.NoError(t, err)

		updateCalled := false
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err = UpdateTargetVersion(versionFile, 0)
		require.NoError(t, err)
		assert.False(t, updateCalled, "update should not be called when versions match")
		assert.False(t, exitCalled, "exit should not be called when no update needed")
	})
}

// TestUpdateTargetVersion_EmptyVersionFile tests UpdateTargetVersion with empty version file path.
func TestUpdateTargetVersion_EmptyVersionFile(t *testing.T) {
	mockey.PatchConvey("empty version file", t, func() {
		updateCalled := false
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err := UpdateTargetVersion("", 0)
		require.NoError(t, err)
		assert.False(t, updateCalled, "update should not be called for empty version file")
		assert.False(t, exitCalled, "exit should not be called for empty version file")
	})
}

// TestUpdateTargetVersion_VersionFileNotExists tests UpdateTargetVersion when version file doesn't exist.
func TestUpdateTargetVersion_VersionFileNotExists(t *testing.T) {
	mockey.PatchConvey("version file not exists", t, func() {
		updateCalled := false
		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			updateCalled = true
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err := UpdateTargetVersion("/nonexistent/version.txt", 0)
		require.NoError(t, err)
		assert.False(t, updateCalled, "update should not be called for missing version file")
		assert.False(t, exitCalled, "exit should not be called for missing version file")
	})
}

// TestUpdateTargetVersion_AutoExitCodeNegative tests UpdateTargetVersion with negative auto exit code.
func TestUpdateTargetVersion_AutoExitCodeNegative(t *testing.T) {
	mockey.PatchConvey("auto exit code negative", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
		require.NoError(t, err)

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			return nil
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err = UpdateTargetVersion(versionFile, -1)
		require.NoError(t, err)
		assert.False(t, exitCalled, "exit should not be called when autoExitCode is -1")
	})
}

// TestUpdateTargetVersion_UpdateError tests UpdateTargetVersion when update fails.
func TestUpdateTargetVersion_UpdateError(t *testing.T) {
	mockey.PatchConvey("update error", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		tempDir := t.TempDir()
		versionFile := filepath.Join(tempDir, "version.txt")
		err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
		require.NoError(t, err)

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			return errors.New("update failed")
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
		}).Build()

		err = UpdateTargetVersion(versionFile, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
		assert.False(t, exitCalled, "exit should not be called when update fails")
	})
}

// TestCheckVersionFileForUpdate_ReadFileError tests checkVersionFileForUpdate with read error.
func TestCheckVersionFileForUpdate_ReadFileError(t *testing.T) {
	mockey.PatchConvey("read file error", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "1.0.0"

		// Mock ReadFile to return an error that is not ErrNotExist
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("read permission denied")
		}).Build()

		ver, needUpdate, err := checkVersionFileForUpdate("/some/path/version.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		assert.Equal(t, "", ver)
		assert.False(t, needUpdate)
	})
}

// TestCheckVersionFileForUpdate_VersionWithLeadingV tests checkVersionFileForUpdate with v prefix.
func TestCheckVersionFileForUpdate_VersionWithLeadingV(t *testing.T) {
	originalVersion := version.Version
	defer func() {
		version.Version = originalVersion
	}()
	version.Version = "v1.0.0"

	tempDir := t.TempDir()
	versionFile := filepath.Join(tempDir, "version.txt")
	err := os.WriteFile(versionFile, []byte("v2.0.0\n"), 0644)
	require.NoError(t, err)

	ver, needUpdate, err := checkVersionFileForUpdate(versionFile)
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", ver)
	assert.True(t, needUpdate)
}

// TestCheckVersionFileForUpdate_VersionWithTabs tests checkVersionFileForUpdate with tabs.
func TestCheckVersionFileForUpdate_VersionWithTabs(t *testing.T) {
	originalVersion := version.Version
	defer func() {
		version.Version = originalVersion
	}()
	version.Version = "1.0.0"

	tempDir := t.TempDir()
	versionFile := filepath.Join(tempDir, "version.txt")
	err := os.WriteFile(versionFile, []byte("\t2.0.0\t\n"), 0644)
	require.NoError(t, err)

	ver, needUpdate, err := checkVersionFileForUpdate(versionFile)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", ver)
	assert.True(t, needUpdate)
}

// TestUpdateTargetVersion_WithDifferentExitCodes tests UpdateTargetVersion with various exit codes.
func TestUpdateTargetVersion_WithDifferentExitCodes(t *testing.T) {
	exitCodes := []int{0, 1, 2, 42, 255}

	for _, expectedCode := range exitCodes {
		t.Run("exit code "+string(rune(expectedCode+'0')), func(t *testing.T) {
			mockey.PatchConvey("exit code test", t, func() {
				originalVersion := version.Version
				defer func() {
					version.Version = originalVersion
				}()
				version.Version = "1.0.0"

				tempDir := t.TempDir()
				versionFile := filepath.Join(tempDir, "version.txt")
				err := os.WriteFile(versionFile, []byte("2.0.0"), 0644)
				require.NoError(t, err)

				mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
					return false, nil
				}).Build()

				mockey.Mock(UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
					return nil
				}).Build()

				var capturedCode int
				mockey.Mock(os.Exit).To(func(code int) {
					capturedCode = code
				}).Build()

				err = UpdateTargetVersion(versionFile, expectedCode)
				require.NoError(t, err)
				assert.Equal(t, expectedCode, capturedCode)
			})
		})
	}
}
