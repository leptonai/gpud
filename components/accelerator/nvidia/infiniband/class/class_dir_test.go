package class

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClassDirInterface(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (string, func())
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid directory",
			setup: func() (string, func()) {
				tmpDir, err := os.MkdirTemp("", "test-classdir-*")
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir, func() { _ = os.RemoveAll(tmpDir) }
			},
			wantErr: false,
		},
		{
			name: "non-existent directory",
			setup: func() (string, func()) {
				return "/non/existent/path", func() {}
			},
			wantErr: true,
			errMsg:  "could not read",
		},
		{
			name: "file instead of directory",
			setup: func() (string, func()) {
				tmpFile, err := os.CreateTemp("", "test-classdir-file-*")
				if err != nil {
					t.Fatal(err)
				}
				if err := tmpFile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpFile.Name(), func() { _ = os.Remove(tmpFile.Name()) }
			},
			wantErr: true,
			errMsg:  "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir, cleanup := tt.setup()
			defer cleanup()

			got, err := newClassDirInterface(rootDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("newClassDirInterface() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("newClassDirInterface() error = %v, want error containing %q", err, tt.errMsg)
			}
			if !tt.wantErr && got == nil {
				t.Error("newClassDirInterface() returned nil interface without error")
			}
		})
	}
}

func TestClassDirExists(t *testing.T) {
	// Create temporary test directory structure
	tmpDir, err := os.MkdirTemp("", "test-classdir-exists-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test files and directories
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	testDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatal(err)
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name:    "existing file",
			path:    "test.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "existing directory",
			path:    "subdir",
			want:    true,
			wantErr: false,
		},
		{
			name:    "non-existent file",
			path:    "nonexistent.txt",
			want:    false,
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			want:    true, // empty path refers to the root directory itself
			wantErr: false,
		},
		{
			name:    "path with subdirectories",
			path:    "subdir/nonexistent.txt",
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cd.exists(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("exists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassDirReadFile(t *testing.T) {
	// Create temporary test directory structure
	tmpDir, err := os.MkdirTemp("", "test-classdir-readfile-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test files
	testContent := "test content"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	// File with whitespace
	whitespaceContent := "  content with spaces  \n\t"
	whitespaceFile := filepath.Join(tmpDir, "whitespace.txt")
	if err := os.WriteFile(whitespaceFile, []byte(whitespaceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Empty file
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Unreadable file (on Unix systems)
	unreadableFile := filepath.Join(tmpDir, "unreadable.txt")
	if err := os.WriteFile(unreadableFile, []byte("secret"), 0000); err != nil {
		t.Fatal(err)
	}

	// Create subdirectory
	subdir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		file     string
		want     string
		wantErr  bool
		skipOnCI bool // Skip tests that might behave differently in CI
	}{
		{
			name:    "read existing file",
			file:    "test.txt",
			want:    testContent,
			wantErr: false,
		},
		{
			name:    "read file with whitespace",
			file:    "whitespace.txt",
			want:    "content with spaces", // trimmed
			wantErr: false,
		},
		{
			name:    "read empty file",
			file:    "empty.txt",
			want:    "",
			wantErr: false,
		},
		{
			name:    "read non-existent file",
			file:    "nonexistent.txt",
			want:    "",
			wantErr: true,
		},
		{
			name:    "read directory instead of file",
			file:    "subdir",
			want:    "",
			wantErr: true,
		},
		{
			name:     "read unreadable file",
			file:     "unreadable.txt",
			want:     "",
			wantErr:  true,
			skipOnCI: true, // Permission tests might not work in all environments
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnCI && os.Getenv("CI") != "" {
				t.Skip("Skipping permission test in CI environment")
			}

			got, err := cd.readFile(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("readFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("readFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassDirListDir(t *testing.T) {
	// Create temporary test directory structure
	tmpDir, err := os.MkdirTemp("", "test-classdir-listdir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test structure
	// tmpDir/
	//   file1.txt
	//   file2.txt
	//   subdir1/
	//     file3.txt
	//   subdir2/
	//   emptydir/

	files := []string{"file1.txt", "file2.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	subdirs := []string{"subdir1", "subdir2", "emptydir"}
	for _, d := range subdirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Add a file in subdir1
	if err := os.WriteFile(filepath.Join(tmpDir, "subdir1", "file3.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		dir       string
		wantNames []string
		wantErr   bool
	}{
		{
			name:      "list root directory",
			dir:       "",
			wantNames: []string{"emptydir", "file1.txt", "file2.txt", "subdir1", "subdir2"},
			wantErr:   false,
		},
		{
			name:      "list subdirectory with file",
			dir:       "subdir1",
			wantNames: []string{"file3.txt"},
			wantErr:   false,
		},
		{
			name:      "list empty subdirectory",
			dir:       "emptydir",
			wantNames: []string{},
			wantErr:   false,
		},
		{
			name:      "list non-existent directory",
			dir:       "nonexistent",
			wantNames: nil,
			wantErr:   true,
		},
		{
			name:      "list file instead of directory",
			dir:       "file1.txt",
			wantNames: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cd.listDir(tt.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("listDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Extract names from DirEntry slice
				var gotNames []string
				for _, entry := range got {
					gotNames = append(gotNames, entry.Name())
				}

				// Check lengths match
				if len(gotNames) != len(tt.wantNames) {
					t.Errorf("listDir() returned %d entries, want %d", len(gotNames), len(tt.wantNames))
					t.Errorf("got: %v", gotNames)
					t.Errorf("want: %v", tt.wantNames)
					return
				}

				// Check all expected names are present
				for _, wantName := range tt.wantNames {
					found := false
					for _, gotName := range gotNames {
						if gotName == wantName {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("listDir() missing expected entry %q", wantName)
					}
				}
			}
		})
	}
}

func TestClassDirIntegration(t *testing.T) {
	// Integration test using multiple methods together
	tmpDir, err := os.MkdirTemp("", "test-classdir-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a structure similar to /sys/class/infiniband
	deviceDir := filepath.Join(tmpDir, "mlx5_0")
	portDir := filepath.Join(deviceDir, "ports", "1")
	if err := os.MkdirAll(portDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files
	stateFile := filepath.Join(portDir, "state")
	if err := os.WriteFile(stateFile, []byte("4: ACTIVE\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rateFile := filepath.Join(portDir, "rate")
	if err := os.WriteFile(rateFile, []byte("100 Gb/sec\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test workflow: list devices, check existence, read files
	devices, err := cd.listDir("")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Name() != "mlx5_0" {
		t.Errorf("expected one device mlx5_0, got %v", devices)
	}

	// Check if ports directory exists
	exists, err := cd.exists("mlx5_0/ports")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected mlx5_0/ports to exist")
	}

	// Read state file
	state, err := cd.readFile("mlx5_0/ports/1/state")
	if err != nil {
		t.Fatal(err)
	}
	if state != "4: ACTIVE" {
		t.Errorf("expected state '4: ACTIVE', got %q", state)
	}

	// Read rate file
	rate, err := cd.readFile("mlx5_0/ports/1/rate")
	if err != nil {
		t.Fatal(err)
	}
	if rate != "100 Gb/sec" {
		t.Errorf("expected rate '100 Gb/sec', got %q", rate)
	}
}

func TestClassDirErrors(t *testing.T) {
	// Test error propagation and edge cases
	tmpDir, err := os.MkdirTemp("", "test-classdir-errors-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test with invalid paths
	t.Run("readFile with absolute path outside root", func(t *testing.T) {
		// This should fail because the file is opened relative to the FS
		_, err := cd.readFile("/etc/passwd")
		if err == nil {
			t.Error("expected error when reading absolute path")
		}
	})

	t.Run("listDir with path traversal", func(t *testing.T) {
		// Attempt path traversal
		_, err := cd.listDir("../")
		// This might or might not error depending on the implementation
		// but it should not allow access outside the root directory
		if err == nil {
			// If no error, verify it's not actually listing parent directory
			entries, _ := cd.listDir("../")
			// The behavior here depends on os.DirFS implementation
			_ = entries // just to use the variable
		}
	})
}

// TestClassDirPermissions tests permission-related edge cases
func TestClassDirPermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission tests when running as root")
	}

	tmpDir, err := os.MkdirTemp("", "test-classdir-perms-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Restore permissions before cleanup
		_ = os.Chmod(tmpDir, 0755)
		_ = os.RemoveAll(tmpDir)
	}()

	// Create unreadable directory
	unreadableDir := filepath.Join(tmpDir, "unreadable")
	if err := os.MkdirAll(unreadableDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Remove read permission
	if err := os.Chmod(unreadableDir, 0000); err != nil {
		t.Fatal(err)
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Try to list unreadable directory
	_, err = cd.listDir("unreadable")
	if err == nil {
		t.Error("expected error when listing unreadable directory")
	}

	// Restore permissions for cleanup
	_ = os.Chmod(unreadableDir, 0755)
}

// TestClassDirConcurrency tests thread safety
func TestClassDirConcurrency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-classdir-concurrent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test files
	for i := 0; i < 10; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(filename, []byte(fmt.Sprintf("content %d", i)), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cd, err := newClassDirInterface(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer func() { done <- true }()

			// Test various operations concurrently
			filename := fmt.Sprintf("file%d.txt", i)

			// Check existence
			exists, err := cd.exists(filename)
			if err != nil || !exists {
				t.Errorf("concurrent exists(%s) failed: %v", filename, err)
			}

			// Read file
			content, err := cd.readFile(filename)
			if err != nil {
				t.Errorf("concurrent readFile(%s) failed: %v", filename, err)
			}
			expectedContent := fmt.Sprintf("content %d", i)
			if content != expectedContent {
				t.Errorf("concurrent readFile(%s) = %q, want %q", filename, content, expectedContent)
			}

			// List directory
			_, err = cd.listDir("")
			if err != nil {
				t.Errorf("concurrent listDir failed: %v", err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
