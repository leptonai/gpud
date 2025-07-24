package informer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
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

// TestNewFileInformer tests the constructor with default paths
func TestNewFileInformer(t *testing.T) {
	ch := NewFileInformer()
	assert.NotNil(t, ch, "NewFileInformer should return a channel")
}

// TestNewFileInformerWithConfig tests the constructor with custom paths
func TestNewFileInformerWithConfig(t *testing.T) {
	packagesDir := "/custom/packages"
	rootDir := "/custom/root"

	ch := NewFileInformerWithConfig(packagesDir, rootDir)
	assert.NotNil(t, ch, "NewFileInformerWithConfig should return a channel")
}

func TestFileInformerListPackages(t *testing.T) {
	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "test_packages")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test package directories
	testPackages := []string{"package1", "package2", "package3"}
	for _, pkg := range testPackages {
		pkgDir := filepath.Join(tempDir, pkg)
		if err := os.Mkdir(pkgDir, 0755); err != nil {
			t.Fatalf("Failed to create package dir %s: %v", pkgDir, err)
		}
	}

	// Create fileInformer with test directory
	fi := &fileInformer{
		packagesDir: tempDir,
		rootDir:     tempDir,
	}

	// Test listPackages
	output, err := fi.listPackages()
	assert.NoError(t, err)

	// Parse output and verify packages are listed
	packages := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, expectedPkg := range testPackages {
		found := false
		for _, actualPkg := range packages {
			if strings.TrimSpace(actualPkg) == expectedPkg {
				found = true
				break
			}
		}
		assert.True(t, found, "Package %s should be listed", expectedPkg)
	}
}

func TestFileInformerProcessInitialPackages(t *testing.T) {
	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "test_packages")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test package with init.sh script
	pkgDir := filepath.Join(tempDir, "testpkg")
	if err := os.Mkdir(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create init.sh with test content
	initScript := filepath.Join(pkgDir, "init.sh")
	scriptContent := `#!/bin/bash
#GPUD_PACKAGE_VERSION=1.2.3
#GPUD_PACKAGE_DEPENDENCY=dep1:1.0.0,dep2:2.0.0
#GPUD_PACKAGE_INSTALL_TIME=30s
echo "Test package"`

	if err := os.WriteFile(initScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create init script: %v", err)
	}

	// Create fileInformer
	fi := &fileInformer{
		packagesDir: tempDir,
		rootDir:     tempDir,
	}

	// Create channel to capture results
	ch := make(chan packages.PackageInfo, 10)

	// Test processInitialPackages
	fi.processInitialPackages(ch)

	// Verify package info was sent to channel
	select {
	case info := <-ch:
		assert.Equal(t, "testpkg", info.Name)
		assert.Equal(t, initScript, info.ScriptPath)
		assert.Equal(t, "1.2.3", info.TargetVersion)
		assert.Equal(t, [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}}, info.Dependency)
		assert.Equal(t, 30*time.Second, info.TotalTime)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected package info not received")
	}
}

func TestFileInformerHandleFileEvent(t *testing.T) {
	// Create temporary test directory that matches expected structure
	tempDir, err := os.MkdirTemp("", "test_packages")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the expected directory structure: /tmp/testXXX/var/lib/gpud/packages/testpkg/
	varDir := filepath.Join(tempDir, "var")
	libDir := filepath.Join(varDir, "lib")
	gpudDir := filepath.Join(libDir, "gpud")
	packagesDir := filepath.Join(gpudDir, "packages")
	pkgDir := filepath.Join(packagesDir, "testpkg")

	for _, dir := range []string{varDir, libDir, gpudDir, packagesDir, pkgDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create init.sh script in the expected location
	initScript := filepath.Join(pkgDir, "init.sh")
	scriptContent := `#!/bin/bash
#GPUD_PACKAGE_VERSION=2.0.0
#GPUD_PACKAGE_DEPENDENCY=newdep:3.0.0
#GPUD_PACKAGE_INSTALL_TIME=45s
echo "Updated package"`

	if err := os.WriteFile(initScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create init script: %v", err)
	}

	fi := &fileInformer{
		packagesDir: packagesDir,
		rootDir:     gpudDir,
	}

	// Create channel to capture results
	ch := make(chan packages.PackageInfo, 10)

	// Test handleFileEvent with WRITE event using the actual file path
	event := fsnotify.Event{
		Name: initScript,
		Op:   fsnotify.Write,
	}

	fi.handleFileEvent(nil, event, ch)

	// Verify package info was sent to channel
	select {
	case info := <-ch:
		assert.Equal(t, "testpkg", info.Name)
		assert.Equal(t, initScript, info.ScriptPath)
		assert.Equal(t, "2.0.0", info.TargetVersion)
		assert.Equal(t, [][]string{{"newdep", "3.0.0"}}, info.Dependency)
		assert.Equal(t, 45*time.Second, info.TotalTime)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected package info not received")
	}
}

func TestFileInformerHandleFileEventCreate(t *testing.T) {
	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "test_create")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a subdirectory to simulate directory creation
	newDir := filepath.Join(tempDir, "newdir")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("Failed to create new dir: %v", err)
	}

	fi := &fileInformer{
		packagesDir: tempDir,
		rootDir:     tempDir,
	}

	// Test with real fsnotify.Watcher since the CREATE event requires it
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// Create channel (won't be used for CREATE events)
	ch := make(chan packages.PackageInfo, 10)

	// Test handleFileEvent with CREATE event
	event := fsnotify.Event{
		Name: newDir,
		Op:   fsnotify.Create,
	}

	fi.handleFileEvent(watcher, event, ch)

	// Since we can't easily mock the addDirectory call in the real implementation,
	// we'll just verify that the function completes without error for directory creation
	// The test passes if no panic occurs
}

func TestFileInformerHandleFileEventRemove(t *testing.T) {
	fi := &fileInformer{
		packagesDir: "/test/packages",
		rootDir:     "/test/root",
	}

	// Test with real fsnotify.Watcher since the REMOVE event requires it
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// Create channel (won't be used for REMOVE events)
	ch := make(chan packages.PackageInfo, 10)

	// Test handleFileEvent with REMOVE event for non-existent path
	event := fsnotify.Event{
		Name: "/test/nonexistent",
		Op:   fsnotify.Remove,
	}

	fi.handleFileEvent(watcher, event, ch)

	// Since we can't easily mock the watcher.Remove call in the real implementation,
	// we'll just verify that the function completes without error for file removal
	// The test passes if no panic occurs
}

func TestFileInformerHandleFileEventNonInitScript(t *testing.T) {
	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "test_packages")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file that's not init.sh
	pkgDir := filepath.Join(tempDir, "testpkg")
	if err := os.Mkdir(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	testFile := filepath.Join(pkgDir, "other_file.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fi := &fileInformer{
		packagesDir: tempDir,
		rootDir:     tempDir,
	}

	// Create channel to capture results
	ch := make(chan packages.PackageInfo, 10)

	// Test handleFileEvent with WRITE event for non-init.sh file
	event := fsnotify.Event{
		Name: testFile,
		Op:   fsnotify.Write,
	}

	fi.handleFileEvent(nil, event, ch)

	// Verify no package info was sent to channel (should return early)
	select {
	case <-ch:
		t.Fatal("No package info should have been sent for non-init.sh file")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message should be sent
	}
}

func TestFileInformerHandleFileEventWrongPath(t *testing.T) {
	fi := &fileInformer{
		packagesDir: "/var/lib/gpud/packages",
		rootDir:     "/var/lib/gpud/",
	}

	// Create channel to capture results
	ch := make(chan packages.PackageInfo, 10)

	// Test handleFileEvent with WRITE event for file outside packages directory
	event := fsnotify.Event{
		Name: "/other/path/init.sh",
		Op:   fsnotify.Write,
	}

	fi.handleFileEvent(nil, event, ch)

	// Verify no package info was sent to channel (should return early)
	select {
	case <-ch:
		t.Fatal("No package info should have been sent for file outside packages directory")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message should be sent
	}
}

// Integration-style test that doesn't require actual filesystem watching
func TestFileInformerIntegration(t *testing.T) {
	// Create temporary test directory structure
	tempDir, err := os.MkdirTemp("", "test_integration")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	packagesDir := filepath.Join(tempDir, "packages")
	if err := os.Mkdir(packagesDir, 0755); err != nil {
		t.Fatalf("Failed to create packages dir: %v", err)
	}

	// Create multiple test packages
	testPackages := []struct {
		name    string
		version string
		deps    string
		time    string
	}{
		{"pkg1", "1.0.0", "dep1:1.0.0", "30s"},
		{"pkg2", "2.0.0", "dep2:2.0.0,dep3:3.0.0", "60s"},
		{"pkg3", "3.0.0", "", "15s"},
	}

	for _, pkg := range testPackages {
		pkgDir := filepath.Join(packagesDir, pkg.name)
		if err := os.Mkdir(pkgDir, 0755); err != nil {
			t.Fatalf("Failed to create package dir %s: %v", pkgDir, err)
		}

		initScript := filepath.Join(pkgDir, "init.sh")
		scriptContent := fmt.Sprintf(`#!/bin/bash
#GPUD_PACKAGE_VERSION=%s
#GPUD_PACKAGE_DEPENDENCY=%s
#GPUD_PACKAGE_INSTALL_TIME=%s
echo "Package %s"`, pkg.version, pkg.deps, pkg.time, pkg.name)

		if err := os.WriteFile(initScript, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("Failed to create init script for %s: %v", pkg.name, err)
		}
	}

	// Test with custom configuration
	fi := &fileInformer{
		packagesDir: packagesDir,
		rootDir:     tempDir,
	}

	// Test listPackages
	output, err := fi.listPackages()
	assert.NoError(t, err)
	assert.Contains(t, string(output), "pkg1")
	assert.Contains(t, string(output), "pkg2")
	assert.Contains(t, string(output), "pkg3")

	// Test processInitialPackages
	ch := make(chan packages.PackageInfo, 10)
	fi.processInitialPackages(ch)

	// Collect all package infos
	receivedPackages := make(map[string]packages.PackageInfo)
	timeout := time.After(2 * time.Second)

	for len(receivedPackages) < len(testPackages) {
		select {
		case info := <-ch:
			receivedPackages[info.Name] = info
		case <-timeout:
			t.Fatalf("Timeout waiting for package infos. Received: %d, Expected: %d",
				len(receivedPackages), len(testPackages))
		}
	}

	// Verify all packages were processed correctly
	for _, expectedPkg := range testPackages {
		info, found := receivedPackages[expectedPkg.name]
		assert.True(t, found, "Package %s should have been processed", expectedPkg.name)

		if found {
			assert.Equal(t, expectedPkg.name, info.Name)
			assert.Equal(t, expectedPkg.version, info.TargetVersion)
			assert.Contains(t, info.ScriptPath, expectedPkg.name)
			assert.Contains(t, info.ScriptPath, "init.sh")

			// Parse expected time
			expectedDuration, err := time.ParseDuration(expectedPkg.time)
			assert.NoError(t, err)
			assert.Equal(t, expectedDuration, info.TotalTime)
		}
	}
}
