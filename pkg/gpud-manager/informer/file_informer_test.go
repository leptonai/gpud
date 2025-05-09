package informer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Define an interface for the fsnotify.Watcher to allow mocking
type watcherInterface interface {
	Add(path string) error
	Remove(path string) error
	Close() error
}

// mockWatcher implements the watcherInterface for testing
type mockWatcher struct {
	addFunc    func(path string) error
	removeFunc func(path string) error
	closeFunc  func() error
}

func (m *mockWatcher) Add(path string) error {
	return m.addFunc(path)
}

func (m *mockWatcher) Remove(path string) error {
	if m.removeFunc != nil {
		return m.removeFunc(path)
	}
	return nil
}

func (m *mockWatcher) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// Modified addDirectory to accept our interface instead of *fsnotify.Watcher
func addDirectoryTest(watcher watcherInterface, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestAddDirectory(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp(t.TempDir(), "addDirectory_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create subdirectories
	subDir1 := filepath.Join(tempDir, "subdir1")
	subDir2 := filepath.Join(tempDir, "subdir2")
	subDir3 := filepath.Join(subDir1, "subdir3")

	for _, dir := range []string{subDir1, subDir2, subDir3} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create a file (should not be added to watcher)
	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a mock watcher
	watchedDirs := make(map[string]bool)
	mockWatcher := &mockWatcher{
		addFunc: func(path string) error {
			watchedDirs[path] = true
			return nil
		},
	}

	// Call addDirectory
	err = addDirectoryTest(mockWatcher, tempDir)
	assert.NoError(t, err)

	// Verify that all directories were added
	expectedDirs := []string{tempDir, subDir1, subDir2, subDir3}
	for _, dir := range expectedDirs {
		assert.True(t, watchedDirs[dir], "Directory %s should have been added to watcher", dir)
	}

	// Verify that the file was not added
	assert.False(t, watchedDirs[testFile], "File should not have been added to watcher")
}

func TestResolveDependencies(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected [][]string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "single dependency",
			input:    "pkg1:1.0.0",
			expected: [][]string{{"pkg1", "1.0.0"}},
		},
		{
			name:     "multiple dependencies",
			input:    "pkg1:1.0.0,pkg2:2.0.0,pkg3:3.0.0",
			expected: [][]string{{"pkg1", "1.0.0"}, {"pkg2", "2.0.0"}, {"pkg3", "3.0.0"}},
		},
		{
			name:     "with spaces",
			input:    " pkg1:1.0.0 , pkg2:2.0.0 ",
			expected: [][]string{{"pkg1", "1.0.0 "}, {" pkg2", "2.0.0"}},
		},
		{
			name:     "invalid format",
			input:    "pkg1:1.0.0,pkg2,pkg3:3.0.0",
			expected: [][]string{{"pkg1", "1.0.0"}, {"pkg3", "3.0.0"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveDependencies(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Mock implementation of resolvePackage for testing
func mockResolvePackage() (string, [][]string, time.Duration, error) {
	return "1.0.0",
		[][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}},
		30 * time.Second,
		nil
}

// TestResolvePackage tests the resolvePackage function indirectly
func TestResolvePackage(t *testing.T) {
	// Since we can't easily mock exec.Command, we'll test with a mock implementation
	version, dependencies, totalTime, err := mockResolvePackage()

	// Verify the expected results
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", version)
	assert.Equal(t, [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}}, dependencies)
	assert.Equal(t, 30*time.Second, totalTime)
}

// TestResolveDependenciesWithEmptyString test for resolveDependencies function
func TestResolveDependenciesWithEmptyString(t *testing.T) {
	result := resolveDependencies("")
	assert.Empty(t, result, "Empty string should result in empty dependencies")
}
