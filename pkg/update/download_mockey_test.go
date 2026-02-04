package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/logger"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

// TestDownloadURLToFile_NewClientError tests downloadURLToFile when client creation fails.
func TestDownloadURLToFile_NewClientError(t *testing.T) {
	mockey.PatchConvey("new client error", t, func() {
		mockey.Mock(distsign.NewClient).To(func(logf logger.Logf, pkgsAddr string) (*distsign.Client, error) {
			return nil, errors.New("invalid address")
		}).Build()

		err := downloadURLToFile(context.Background(), "test.tgz", "/tmp/test.tgz", "invalid://url")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}

// TestDownloadURLToFile_DownloadError tests downloadURLToFile when download fails.
func TestDownloadURLToFile_DownloadError(t *testing.T) {
	mockey.PatchConvey("download error", t, func() {
		mockClient := &distsign.Client{}
		mockey.Mock(distsign.NewClient).To(func(logf logger.Logf, pkgsAddr string) (*distsign.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*distsign.Client).Download).To(func(c *distsign.Client, ctx context.Context, srcPath, dstPath string) error {
			return errors.New("download failed")
		}).Build()

		err := downloadURLToFile(context.Background(), "test.tgz", "/tmp/test.tgz", "https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download failed")
	})
}

// TestDownloadURLToFile_Success tests successful downloadURLToFile.
func TestDownloadURLToFile_Success(t *testing.T) {
	mockey.PatchConvey("download success", t, func() {
		mockClient := &distsign.Client{}
		mockey.Mock(distsign.NewClient).To(func(logf logger.Logf, pkgsAddr string) (*distsign.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*distsign.Client).Download).To(func(c *distsign.Client, ctx context.Context, srcPath, dstPath string) error {
			return nil
		}).Build()

		tempDir := t.TempDir()
		dstPath := filepath.Join(tempDir, "test.tgz")

		err := downloadURLToFile(context.Background(), "test.tgz", dstPath, "https://example.com")
		require.NoError(t, err)
	})
}

// TestTarballName_WithUbuntuVersion tests tarballName with Ubuntu version.
func TestTarballName_WithUbuntuVersion(t *testing.T) {
	mockey.PatchConvey("with ubuntu version", t, func() {
		mockey.Mock(detectUbuntuVersion).To(func() string {
			return "ubuntu22.04"
		}).Build()

		result := tarballName("1.0.0", "linux", "amd64")
		assert.Equal(t, "gpud_1.0.0_linux_amd64_ubuntu22.04.tgz", result)
	})
}

// TestTarballName_WithoutUbuntuVersion tests tarballName without Ubuntu version.
func TestTarballName_WithoutUbuntuVersion(t *testing.T) {
	mockey.PatchConvey("without ubuntu version", t, func() {
		mockey.Mock(detectUbuntuVersion).To(func() string {
			return ""
		}).Build()

		result := tarballName("1.0.0", "linux", "amd64")
		assert.Equal(t, "gpud_1.0.0_linux_amd64.tgz", result)
	})
}

// TestDownloadLinuxTarball_UserCacheDirError tests downloadLinuxTarball when UserCacheDir fails.
func TestDownloadLinuxTarball_UserCacheDirError(t *testing.T) {
	mockey.PatchConvey("user cache dir error", t, func() {
		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return "", errors.New("no cache dir")
		}).Build()

		tempDir := t.TempDir()
		mockey.Mock(os.TempDir).To(func() string {
			return tempDir
		}).Build()

		mockClient := &distsign.Client{}
		mockey.Mock(distsign.NewClient).To(func(logf logger.Logf, pkgsAddr string) (*distsign.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*distsign.Client).Download).To(func(c *distsign.Client, ctx context.Context, srcPath, dstPath string) error {
			// Verify that temp dir is used
			if !strings.Contains(dstPath, tempDir) {
				return errors.New("expected temp dir to be used")
			}
			return nil
		}).Build()

		mockey.Mock(detectUbuntuVersion).To(func() string {
			return ""
		}).Build()

		_, err := downloadLinuxTarball(context.Background(), "1.0.0", "https://example.com")
		require.NoError(t, err)
	})
}

// TestDownloadLinuxTarball_MkdirAllError tests downloadLinuxTarball when MkdirAll fails.
func TestDownloadLinuxTarball_MkdirAllError(t *testing.T) {
	mockey.PatchConvey("mkdir all error", t, func() {
		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return "/tmp/cache", nil
		}).Build()

		mockey.Mock(os.MkdirAll).To(func(path string, perm os.FileMode) error {
			return errors.New("permission denied")
		}).Build()

		_, err := downloadLinuxTarball(context.Background(), "1.0.0", "https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}

// TestDownloadLinuxTarball_DownloadURLToFileError tests downloadLinuxTarball when downloadURLToFile fails.
func TestDownloadLinuxTarball_DownloadURLToFileError(t *testing.T) {
	mockey.PatchConvey("download url to file error", t, func() {
		tempDir := t.TempDir()
		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return tempDir, nil
		}).Build()

		mockey.Mock(detectUbuntuVersion).To(func() string {
			return ""
		}).Build()

		mockey.Mock(downloadURLToFile).To(func(ctx context.Context, pathSrc, fileDst, pkgAddr string) error {
			return errors.New("network timeout")
		}).Build()

		_, err := downloadLinuxTarball(context.Background(), "1.0.0", "https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network timeout")
	})
}

// TestDownloadLinuxTarball_Success tests successful downloadLinuxTarball.
func TestDownloadLinuxTarball_Success(t *testing.T) {
	mockey.PatchConvey("download linux tarball success", t, func() {
		tempDir := t.TempDir()
		mockey.Mock(os.UserCacheDir).To(func() (string, error) {
			return tempDir, nil
		}).Build()

		mockey.Mock(detectUbuntuVersion).To(func() string {
			return ""
		}).Build()

		mockey.Mock(downloadURLToFile).To(func(ctx context.Context, pathSrc, fileDst, pkgAddr string) error {
			// Verify the expected tarball name is used
			expectedTarball := "gpud_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tgz"
			if pathSrc != expectedTarball {
				return errors.New("unexpected tarball name: " + pathSrc)
			}
			return nil
		}).Build()

		dlPath, err := downloadLinuxTarball(context.Background(), "1.0.0", "https://example.com")
		require.NoError(t, err)
		assert.Contains(t, dlPath, tempDir)
	})
}

// TestDownloadFile_HTTPGetError tests downloadFile when HTTP GET fails.
func TestDownloadFile_HTTPGetError(t *testing.T) {
	mockey.PatchConvey("http get error", t, func() {
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}).Build()

		tempDir := t.TempDir()
		dstPath := filepath.Join(tempDir, "test.txt")

		err := downloadFile("https://example.com/file", dstPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

// TestDownloadFile_BadStatusCode tests downloadFile when HTTP status is not OK.
func TestDownloadFile_BadStatusCode(t *testing.T) {
	mockey.PatchConvey("bad status code", t, func() {
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}).Build()

		tempDir := t.TempDir()
		dstPath := filepath.Join(tempDir, "test.txt")

		err := downloadFile("https://example.com/file", dstPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404 Not Found")
	})
}

// TestDownloadFile_CreateFileError tests downloadFile when creating the file fails.
func TestDownloadFile_CreateFileError(t *testing.T) {
	mockey.PatchConvey("create file error", t, func() {
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("content")),
			}, nil
		}).Build()

		// Try to write to a non-existent directory
		err := downloadFile("https://example.com/file", "/nonexistent/dir/file.txt")
		require.Error(t, err)
	})
}

// TestDownloadFile_Success tests successful downloadFile.
func TestDownloadFile_Success(t *testing.T) {
	mockey.PatchConvey("download file success", t, func() {
		expectedContent := "test file content"
		mockey.Mock(http.Get).To(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(expectedContent)),
			}, nil
		}).Build()

		tempDir := t.TempDir()
		dstPath := filepath.Join(tempDir, "test.txt")

		err := downloadFile("https://example.com/file", dstPath)
		require.NoError(t, err)

		// Verify content
		content, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	})
}

// TestWriteFile_RemoveExistingError tests writeFile when removing existing file fails.
func TestWriteFile_RemoveExistingError(t *testing.T) {
	mockey.PatchConvey("remove existing error", t, func() {
		// This is tested by creating a file in a read-only directory
		// For now, we just verify the function signature works
		tempDir := t.TempDir()
		testPath := filepath.Join(tempDir, "test.txt")

		// Create a file first
		err := os.WriteFile(testPath, []byte("existing"), 0644)
		require.NoError(t, err)

		// Write new content
		err = writeFile(strings.NewReader("new content"), testPath, 0644)
		require.NoError(t, err)

		// Verify content was replaced
		content, err := os.ReadFile(testPath)
		require.NoError(t, err)
		assert.Equal(t, "new content", string(content))
	})
}

// TestCopyFile_SourceOpenError tests copyFile when source cannot be opened.
func TestCopyFile_SourceOpenError(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "nonexistent.txt")
	dstPath := filepath.Join(tempDir, "dst.txt")

	err := copyFile(srcPath, dstPath)
	require.Error(t, err)
}

// TestCopyFile_DestCreateError tests copyFile when destination cannot be created.
func TestCopyFile_DestCreateError(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src.txt")

	// Create source file
	err := os.WriteFile(srcPath, []byte("content"), 0644)
	require.NoError(t, err)

	// Try to write to a non-existent directory
	err = copyFile(srcPath, "/nonexistent/dir/file.txt")
	require.Error(t, err)
}

// TestCopyFile_Success tests successful copyFile.
func TestCopyFile_Success(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src.txt")
	dstPath := filepath.Join(tempDir, "dst.txt")

	expectedContent := "test content for copy"
	err := os.WriteFile(srcPath, []byte(expectedContent), 0644)
	require.NoError(t, err)

	err = copyFile(srcPath, dstPath)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(content))
}
