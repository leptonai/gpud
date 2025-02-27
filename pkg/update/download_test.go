package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func Test_tarballName(t *testing.T) {
	ubuntuVer := detectUbuntuVersion()
	ubuntuSuffix := ""
	if ubuntuVer != "" {
		ubuntuSuffix = "_" + ubuntuVer
	}
	name := tarballName("v0.0.1", "linux", "amd64")
	want := fmt.Sprintf("gpud_v0.0.1_linux_amd64%s.tgz", ubuntuSuffix)
	if name != want {
		t.Fatalf("want: %s, got: %s", want, name)
	}
}

func Test_writeFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "writeFileTest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file path
	testFilePath := filepath.Join(tempDir, "testFile")

	// Test case 1: Writing to a new file
	testContent := []byte("test content")
	err = writeFile(bytes.NewReader(testContent), testFilePath, 0644)
	if err != nil {
		t.Fatalf("writeFile failed: %v", err)
	}

	// Verify the file content
	content, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	if !bytes.Equal(content, testContent) {
		t.Fatalf("file content mismatch; got %q, want %q", content, testContent)
	}

	// Test case 2: Overwriting an existing file
	newContent := []byte("new content")
	err = writeFile(bytes.NewReader(newContent), testFilePath, 0644)
	if err != nil {
		t.Fatalf("writeFile failed on overwrite: %v", err)
	}

	// Verify the new content
	content, err = os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("failed to read test file after overwrite: %v", err)
	}
	if !bytes.Equal(content, newContent) {
		t.Fatalf("file content mismatch after overwrite; got %q, want %q", content, newContent)
	}
}

func Test_copyFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "copyFileTest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a source file with test content
	srcPath := filepath.Join(tempDir, "srcFile")
	srcContent := []byte("source file content")
	if err := os.WriteFile(srcPath, srcContent, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Set up destination path
	dstPath := filepath.Join(tempDir, "dstFile")

	// Copy the file
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify the destination file has the correct content
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if !bytes.Equal(dstContent, srcContent) {
		t.Fatalf("destination file content mismatch; got %q, want %q", dstContent, srcContent)
	}

	// Test error case: non-existent source file
	nonExistPath := filepath.Join(tempDir, "nonExistent")
	if err := copyFile(nonExistPath, dstPath); err == nil {
		t.Fatalf("expected an error when copying from non-existent file, got nil")
	}
}

func Test_downloadFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "downloadFileTest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create mock content to serve
	mockContent := []byte("mock server response")

	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mockContent)
		} else if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
		}
	}))
	defer server.Close()

	// Test case 1: Successful download
	downloadPath := filepath.Join(tempDir, "downloadedFile")
	err = downloadFile(server.URL+"/success", downloadPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the downloaded content
	content, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if !bytes.Equal(content, mockContent) {
		t.Fatalf("downloaded content mismatch; got %q, want %q", content, mockContent)
	}

	// Test case 2: Server error
	err = downloadFile(server.URL+"/error", downloadPath+".error")
	if err == nil {
		t.Fatalf("expected an error from server, got nil")
	}
}

func createMockTarball(t *testing.T, path string) {
	t.Helper()

	// Create a buffer to write our tar file to
	var buf bytes.Buffer

	// Create a gzip writer
	gw := gzip.NewWriter(&buf)

	// Create a tar writer
	tw := tar.NewWriter(gw)

	// Add a file to the archive
	content := []byte("binary file content")
	hdr := &tar.Header{
		Name: "gpud", // This is the file we specifically look for in unpackLinuxTarball
		Mode: 0755,
		Size: int64(len(content)),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}

	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}

	// Close to flush and complete the tar archive
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}

	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	// Write the buffer to the file
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write tarball file: %v", err)
	}
}

func Test_unpackLinuxTarball(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "unpackTarballTest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create paths for the tarball and the file to replace
	tarballPath := filepath.Join(tempDir, "test.tgz")
	fileToReplace := filepath.Join(tempDir, "gpud")

	// Create a mock tarball with the necessary structure
	createMockTarball(t, tarballPath)

	// Test unpacking the tarball
	err = unpackLinuxTarball(tarballPath, fileToReplace)
	if err != nil {
		t.Fatalf("unpackLinuxTarball failed: %v", err)
	}

	// Verify the file was extracted correctly
	if _, err := os.Stat(fileToReplace); os.IsNotExist(err) {
		t.Fatalf("target file was not created: %v", err)
	}

	// Check the permission on the file
	info, err := os.Stat(fileToReplace)
	if err != nil {
		t.Fatalf("failed to stat extracted file: %v", err)
	}

	// On Unix systems, check the file mode (permission bits)
	if info.Mode().Perm() != 0755 {
		t.Fatalf("extracted file has unexpected permissions: got %o, want %o",
			info.Mode().Perm(), 0755)
	}

	// Test case with invalid tarball
	invalidTarballPath := filepath.Join(tempDir, "invalid.tgz")
	if err := os.WriteFile(invalidTarballPath, []byte("not a tarball"), 0644); err != nil {
		t.Fatalf("failed to create invalid tarball: %v", err)
	}

	err = unpackLinuxTarball(invalidTarballPath, fileToReplace)
	if err == nil {
		t.Fatalf("expected an error with invalid tarball, got nil")
	}
}
