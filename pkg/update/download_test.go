package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTarballName(t *testing.T) {
	// Test with various OS/arch combinations
	expectedSuffix := ""

	// Get the Ubuntu version if on Ubuntu
	ubuntuVersion := detectUbuntuVersion()
	if ubuntuVersion != "" {
		expectedSuffix = "_" + ubuntuVersion
	}

	testCases := []struct {
		version  string
		os       string
		arch     string
		expected string
	}{
		{"1.0.0", "linux", "amd64", "gpud_1.0.0_linux_amd64" + expectedSuffix + ".tgz"},
		{"0.5.2", "darwin", "arm64", "gpud_0.5.2_darwin_arm64" + expectedSuffix + ".tgz"},
		{"2.1.3", "windows", "amd64", "gpud_2.1.3_windows_amd64" + expectedSuffix + ".tgz"},
	}

	for _, tc := range testCases {
		result := tarballName(tc.version, tc.os, tc.arch)
		if result != tc.expected {
			t.Errorf("tarballName(%q, %q, %q) = %q, want %q",
				tc.version, tc.os, tc.arch, result, tc.expected)
		}
	}
}

func TestWriteFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp(t.TempDir(), "gpud-writeFile-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	// Test writing file
	err = writeFile(bytes.NewBufferString(testContent), testFile, 0644)
	if err != nil {
		t.Fatalf("writeFile failed: %v", err)
	}

	// Read back and verify
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
	}

	// Test writing to existing file (should succeed by replacing)
	newContent := "new content"
	err = writeFile(bytes.NewBufferString(newContent), testFile, 0644)
	if err != nil {
		t.Fatalf("writeFile failed on existing file: %v", err)
	}

	// Read back and verify again
	content, err = os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file after rewrite: %v", err)
	}
	if string(content) != newContent {
		t.Errorf("Content mismatch after rewrite: got %q, want %q", string(content), newContent)
	}
}

func TestDownloadFile(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test content"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp(t.TempDir(), "gpud-downloadFile-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Test successful download
	successPath := filepath.Join(tempDir, "success.txt")
	err = downloadFile(ts.URL+"/success", successPath)
	if err != nil {
		t.Errorf("downloadFile failed for success path: %v", err)
	} else {
		// Verify file content
		content, err := os.ReadFile(successPath)
		if err != nil {
			t.Fatalf("Failed to read downloaded file: %v", err)
		}
		if string(content) != "test content" {
			t.Errorf("Downloaded content mismatch: got %q, want %q",
				string(content), "test content")
		}
	}

	// Test 404 response
	notFoundPath := filepath.Join(tempDir, "notfound.txt")
	err = downloadFile(ts.URL+"/notfound", notFoundPath)
	if err == nil {
		t.Error("downloadFile should have failed for 404 response")
	}
}

func TestCopyFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp(t.TempDir(), "gpud-copyFile-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	srcContent := "test content for copy"
	err = os.WriteFile(srcPath, []byte(srcContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copying to destination
	dstPath := filepath.Join(tempDir, "destination.txt")
	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("copyFile failed: %v", err)
	} else {
		// Verify file content
		content, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("Failed to read copied file: %v", err)
		}
		if string(content) != srcContent {
			t.Errorf("Copied content mismatch: got %q, want %q",
				string(content), srcContent)
		}
	}

	// Test copying from non-existent source
	err = copyFile(filepath.Join(tempDir, "nonexistent.txt"), dstPath)
	if err == nil {
		t.Error("copyFile should have failed for non-existent source")
	}
}

func TestDownloadLinuxTarball(t *testing.T) {
	// Skip test if not on linux (to avoid unexpected behavior)
	if runtime.GOOS != "linux" {
		t.Skip("Test only applicable on Linux")
	}

	// Test with a context that's already been canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := downloadLinuxTarball(ctx, "0.0.1", DefaultUpdateURL)
	// This should fail because the context is canceled
	if err == nil {
		t.Error("Expected error when downloadLinuxTarball with canceled context")
	}
}

func createTestTarball(t *testing.T, path string) {
	// Create a gzipped tarball for testing
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test tarball: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		_ = gw.Close()
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		_ = tw.Close()
	}()

	// Add a dummy gpud file
	hdr := &tar.Header{
		Name: "gpud",
		Mode: 0755,
		Size: int64(len("dummy gpud binary")),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tw.Write([]byte("dummy gpud binary")); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}
}

func TestUnpackLinuxTarball(t *testing.T) {
	tempDir, err := os.MkdirTemp(t.TempDir(), "gpud-unpackTarball-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test tarball
	tarballPath := filepath.Join(tempDir, "test.tgz")
	createTestTarball(t, tarballPath)

	// Test unpacking
	gpudPath := filepath.Join(tempDir, "gpud")
	err = unpackLinuxTarball(tarballPath, gpudPath)
	if err != nil {
		t.Errorf("unpackLinuxTarball failed: %v", err)
	} else {
		// Verify file exists
		if _, err := os.Stat(gpudPath); os.IsNotExist(err) {
			t.Error("Unpacked gpud binary not found")
		}

		// Verify file content
		content, err := os.ReadFile(gpudPath)
		if err != nil {
			t.Fatalf("Failed to read unpacked file: %v", err)
		}
		if string(content) != "dummy gpud binary" {
			t.Errorf("Unpacked content mismatch: got %q, want %q",
				string(content), "dummy gpud binary")
		}
	}

	// Test with non-existent tarball
	err = unpackLinuxTarball(filepath.Join(tempDir, "nonexistent.tgz"), gpudPath)
	if err == nil {
		t.Error("unpackLinuxTarball should have failed for non-existent tarball")
	}
}
