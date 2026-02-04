package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/osutil"
)

// TestUpdateExecutable_RequireRootError tests UpdateExecutable when root is required but user is not root.
func TestUpdateExecutable_RequireRootError(t *testing.T) {
	mockey.PatchConvey("require root error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return errors.New("this command must be run as root")
		}).Build()

		err := UpdateExecutable("1.0.0", DefaultUpdateURL, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "root")
	})
}

// TestUpdateExecutable_DownloadError tests UpdateExecutable when download fails.
func TestUpdateExecutable_DownloadError(t *testing.T) {
	mockey.PatchConvey("download error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(downloadLinuxTarball).To(func(ctx context.Context, ver, pkgAddr string) (string, error) {
			return "", errors.New("failed to download tarball")
		}).Build()

		err := UpdateExecutable("1.0.0", DefaultUpdateURL, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download")
	})
}

// TestUpdateExecutable_UnpackError tests UpdateExecutable when unpacking fails.
func TestUpdateExecutable_UnpackError(t *testing.T) {
	mockey.PatchConvey("unpack error", t, func() {
		tempDir := t.TempDir()
		dlPath := filepath.Join(tempDir, "test.tgz")

		// Create a dummy file to represent the downloaded tarball
		err := os.WriteFile(dlPath, []byte("dummy content"), 0644)
		require.NoError(t, err)

		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(downloadLinuxTarball).To(func(ctx context.Context, ver, pkgAddr string) (string, error) {
			return dlPath, nil
		}).Build()

		mockey.Mock(os.Executable).To(func() (string, error) {
			return filepath.Join(tempDir, "gpud"), nil
		}).Build()

		mockey.Mock(unpackLinuxTarball).To(func(fileToUnpack string, fileToReplace string) error {
			return errors.New("failed to unpack tarball")
		}).Build()

		err = UpdateExecutable("1.0.0", DefaultUpdateURL, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unpack")
	})
}

// TestUpdateExecutable_ExecutableError tests UpdateExecutable when getting executable path fails.
func TestUpdateExecutable_ExecutableError(t *testing.T) {
	mockey.PatchConvey("executable error", t, func() {
		tempDir := t.TempDir()
		dlPath := filepath.Join(tempDir, "test.tgz")

		err := os.WriteFile(dlPath, []byte("dummy content"), 0644)
		require.NoError(t, err)

		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(downloadLinuxTarball).To(func(ctx context.Context, ver, pkgAddr string) (string, error) {
			return dlPath, nil
		}).Build()

		mockey.Mock(os.Executable).To(func() (string, error) {
			return "", errors.New("failed to get executable path")
		}).Build()

		err = UpdateExecutable("1.0.0", DefaultUpdateURL, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get executable")
	})
}

// TestUpdateExecutable_Success tests successful UpdateExecutable.
func TestUpdateExecutable_Success(t *testing.T) {
	mockey.PatchConvey("update success", t, func() {
		tempDir := t.TempDir()
		dlPath := filepath.Join(tempDir, "test.tgz")

		err := os.WriteFile(dlPath, []byte("dummy content"), 0644)
		require.NoError(t, err)

		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(downloadLinuxTarball).To(func(ctx context.Context, ver, pkgAddr string) (string, error) {
			return dlPath, nil
		}).Build()

		mockey.Mock(os.Executable).To(func() (string, error) {
			return filepath.Join(tempDir, "gpud"), nil
		}).Build()

		mockey.Mock(unpackLinuxTarball).To(func(fileToUnpack string, fileToReplace string) error {
			return nil
		}).Build()

		err = UpdateExecutable("1.0.0", DefaultUpdateURL, true)
		require.NoError(t, err)
	})
}

// TestUpdateExecutable_NoRequireRoot tests UpdateExecutable when root is not required.
func TestUpdateExecutable_NoRequireRoot(t *testing.T) {
	mockey.PatchConvey("no require root", t, func() {
		tempDir := t.TempDir()
		dlPath := filepath.Join(tempDir, "test.tgz")

		err := os.WriteFile(dlPath, []byte("dummy content"), 0644)
		require.NoError(t, err)

		requireRootCalled := false
		mockey.Mock(osutil.RequireRoot).To(func() error {
			requireRootCalled = true
			return nil
		}).Build()

		mockey.Mock(downloadLinuxTarball).To(func(ctx context.Context, ver, pkgAddr string) (string, error) {
			return dlPath, nil
		}).Build()

		mockey.Mock(os.Executable).To(func() (string, error) {
			return filepath.Join(tempDir, "gpud"), nil
		}).Build()

		mockey.Mock(unpackLinuxTarball).To(func(fileToUnpack string, fileToReplace string) error {
			return nil
		}).Build()

		err = UpdateExecutable("1.0.0", DefaultUpdateURL, false)
		require.NoError(t, err)
		assert.False(t, requireRootCalled, "RequireRoot should not be called when requireRoot is false")
	})
}

// TestPackageUpdate_ResolveDataDirError tests PackageUpdate when data dir resolution fails.
func TestPackageUpdate_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("resolve data dir error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		err := PackageUpdate("test-pkg", "1.0.0", DefaultUpdateURL, "/invalid/path")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestPackageUpdate_Success tests successful PackageUpdate.
func TestPackageUpdate_Success(t *testing.T) {
	mockey.PatchConvey("package update success", t, func() {
		tempDir := t.TempDir()
		dlDir := filepath.Join(tempDir, "cache")
		dataDir := filepath.Join(tempDir, "data")

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return dataDir, nil
		}).Build()

		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return dlDir, nil
		}).Build()

		mockey.Mock(downloadFile).To(func(url, filepath string) error {
			// Create a dummy file
			return os.WriteFile(filepath, []byte("test content"), 0644)
		}).Build()

		err := PackageUpdate("test-pkg", "1.0.0", DefaultUpdateURL, dataDir)
		require.NoError(t, err)

		// Verify the file was copied to the correct location
		targetPath := filepath.Join(config.PackagesDir(dataDir), "test-pkg", "init.sh")
		_, err = os.Stat(targetPath)
		require.NoError(t, err)
	})
}

// TestPackageUpdate_UserCacheDirError tests PackageUpdate when UserCacheDir fails.
func TestPackageUpdate_UserCacheDirError(t *testing.T) {
	mockey.PatchConvey("user cache dir error falls back to temp dir", t, func() {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		mockey.Mock(config.ResolveDataDir).To(func(dir string) (string, error) {
			return dataDir, nil
		}).Build()

		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return "", errors.New("no cache dir")
		}).Build()

		mockey.Mock(downloadFile).To(func(url, filepath string) error {
			return os.WriteFile(filepath, []byte("test content"), 0644)
		}).Build()

		err := PackageUpdate("test-pkg", "1.0.0", DefaultUpdateURL, dataDir)
		require.NoError(t, err)
	})
}

// TestPackageUpdate_DownloadError tests PackageUpdate when download fails.
func TestPackageUpdate_DownloadError(t *testing.T) {
	mockey.PatchConvey("download error", t, func() {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")
		dlDir := filepath.Join(tempDir, "cache")

		mockey.Mock(config.ResolveDataDir).To(func(dir string) (string, error) {
			return dataDir, nil
		}).Build()

		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return dlDir, nil
		}).Build()

		mockey.Mock(downloadFile).To(func(url, filepath string) error {
			return errors.New("network error")
		}).Build()

		err := PackageUpdate("test-pkg", "1.0.0", DefaultUpdateURL, dataDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})
}

// TestPackageUpdate_CopyFileError tests PackageUpdate when copy fails.
func TestPackageUpdate_CopyFileError(t *testing.T) {
	mockey.PatchConvey("copy file error", t, func() {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")
		dlDir := filepath.Join(tempDir, "cache")

		mockey.Mock(config.ResolveDataDir).To(func(dir string) (string, error) {
			return dataDir, nil
		}).Build()

		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return dlDir, nil
		}).Build()

		mockey.Mock(downloadFile).To(func(url, filepath string) error {
			return os.WriteFile(filepath, []byte("test content"), 0644)
		}).Build()

		mockey.Mock(copyFile).To(func(src, dst string) error {
			return errors.New("permission denied")
		}).Build()

		err := PackageUpdate("test-pkg", "1.0.0", DefaultUpdateURL, dataDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}
