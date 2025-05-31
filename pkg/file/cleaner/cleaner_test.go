package cleaner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatedAt(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		// Create a temporary file
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test_file.txt")

		// Write some content to the file
		content := []byte("test content")
		err := os.WriteFile(tmpFile, content, 0644)
		require.NoError(t, err, "Failed to create test file")

		// Get the file's modification time using UpdatedAt
		modTime := UpdatedAt(tmpFile)

		// Verify it's not zero time
		assert.False(t, modTime.IsZero(), "UpdatedAt() should not return zero time for existing file")

		// Get the actual modification time for comparison
		stat, err := os.Stat(tmpFile)
		require.NoError(t, err, "Failed to stat test file")

		// Compare the times (should be equal)
		assert.True(t, modTime.Equal(stat.ModTime()), "UpdatedAt() should return the same time as os.Stat().ModTime()")
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := "/path/that/does/not/exist/file.txt"

		modTime := UpdatedAt(nonExistentFile)

		// Should return zero time for non-existent file
		assert.True(t, modTime.IsZero(), "UpdatedAt() should return zero time for non-existent file")
	})

	t.Run("directory", func(t *testing.T) {
		// Create a temporary directory
		tmpDir := t.TempDir()

		// Get the directory's modification time
		modTime := UpdatedAt(tmpDir)

		// Should not return zero time for existing directory
		assert.False(t, modTime.IsZero(), "UpdatedAt() should not return zero time for existing directory")

		// Verify against actual stat
		stat, err := os.Stat(tmpDir)
		require.NoError(t, err, "Failed to stat test directory")

		assert.True(t, modTime.Equal(stat.ModTime()), "UpdatedAt() should return the same time as os.Stat().ModTime() for directory")
	})

	t.Run("file modification time changes", func(t *testing.T) {
		// Create a temporary file
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test_file.txt")

		// Write initial content
		err := os.WriteFile(tmpFile, []byte("initial content"), 0644)
		require.NoError(t, err, "Failed to create test file")

		// Get initial modification time
		initialModTime := UpdatedAt(tmpFile)
		assert.False(t, initialModTime.IsZero(), "Initial modification time should not be zero")

		// Wait a bit to ensure time difference
		time.Sleep(10 * time.Millisecond)

		// Modify the file
		err = os.WriteFile(tmpFile, []byte("modified content"), 0644)
		require.NoError(t, err, "Failed to modify test file")

		// Get new modification time
		newModTime := UpdatedAt(tmpFile)

		// New modification time should be after the initial one
		assert.True(t, newModTime.After(initialModTime), "UpdatedAt() after modification should be after initial time")
	})

	t.Run("empty file path", func(t *testing.T) {
		modTime := UpdatedAt("")

		// Should return zero time for empty path
		assert.True(t, modTime.IsZero(), "UpdatedAt() should return zero time for empty path")
	})

	t.Run("permission denied", func(t *testing.T) {
		// This test might not work on all systems, so we'll skip it if we can't create the scenario
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test_file.txt")

		// Create a file
		err := os.WriteFile(tmpFile, []byte("test"), 0644)
		require.NoError(t, err, "Failed to create test file")

		// Try to make the parent directory inaccessible (this might not work on all systems)
		err = os.Chmod(tmpDir, 0000)
		if err != nil {
			t.Skip("Cannot change directory permissions on this system")
		}

		// Restore permissions after test
		defer func() {
			_ = os.Chmod(tmpDir, 0755)
		}()

		modTime := UpdatedAt(tmpFile)

		// Should return zero time when there's a permission error
		assert.True(t, modTime.IsZero(), "UpdatedAt() should return zero time for permission denied")
	})
}

func TestClean(t *testing.T) {
	t.Run("clean files before timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test files with different timestamps
		oldFile1 := filepath.Join(tmpDir, "old1.txt")
		oldFile2 := filepath.Join(tmpDir, "old2.txt")
		newFile := filepath.Join(tmpDir, "new.txt")

		// Create old files
		err := os.WriteFile(oldFile1, []byte("old content 1"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(oldFile2, []byte("old content 2"), 0644)
		require.NoError(t, err)

		// Set old timestamps (1 hour ago)
		oldTime := time.Now().Add(-1 * time.Hour)
		err = os.Chtimes(oldFile1, oldTime, oldTime)
		require.NoError(t, err)
		err = os.Chtimes(oldFile2, oldTime, oldTime)
		require.NoError(t, err)

		// Wait a bit and create new file
		time.Sleep(10 * time.Millisecond)
		err = os.WriteFile(newFile, []byte("new content"), 0644)
		require.NoError(t, err)

		// Clean files before 30 minutes ago
		cutoffTime := time.Now().Add(-30 * time.Minute)
		pattern := filepath.Join(tmpDir, "*.txt")
		err = Clean(pattern, cutoffTime)
		require.NoError(t, err)

		// Check that old files are removed
		_, err = os.Stat(oldFile1)
		assert.True(t, os.IsNotExist(err), "old file 1 should be removed")
		_, err = os.Stat(oldFile2)
		assert.True(t, os.IsNotExist(err), "old file 2 should be removed")

		// Check that new file still exists
		_, err = os.Stat(newFile)
		assert.NoError(t, err, "new file should still exist")
	})

	t.Run("no files match pattern", func(t *testing.T) {
		tmpDir := t.TempDir()
		pattern := filepath.Join(tmpDir, "*.nonexistent")

		err := Clean(pattern, time.Now())
		assert.NoError(t, err, "Clean should not error when no files match pattern")
	})

	t.Run("invalid pattern", func(t *testing.T) {
		// Use an invalid glob pattern
		pattern := "["

		err := Clean(pattern, time.Now())
		assert.Error(t, err, "Clean should return error for invalid pattern")
	})

	t.Run("clean directories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test directories
		oldDir := filepath.Join(tmpDir, "old_dir")
		newDir := filepath.Join(tmpDir, "new_dir")

		err := os.Mkdir(oldDir, 0755)
		require.NoError(t, err)
		err = os.Mkdir(newDir, 0755)
		require.NoError(t, err)

		// Set old timestamp for old directory
		oldTime := time.Now().Add(-1 * time.Hour)
		err = os.Chtimes(oldDir, oldTime, oldTime)
		require.NoError(t, err)

		// Clean directories before 30 minutes ago
		cutoffTime := time.Now().Add(-30 * time.Minute)
		pattern := filepath.Join(tmpDir, "*_dir")
		err = Clean(pattern, cutoffTime)
		require.NoError(t, err)

		// Check that old directory is removed
		_, err = os.Stat(oldDir)
		assert.True(t, os.IsNotExist(err), "old directory should be removed")

		// Check that new directory still exists
		_, err = os.Stat(newDir)
		assert.NoError(t, err, "new directory should still exist")
	})
}

func TestCleanInternal(t *testing.T) {
	t.Run("clean with custom remove function", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test files
		file1 := filepath.Join(tmpDir, "file1.txt")
		file2 := filepath.Join(tmpDir, "file2.txt")

		err := os.WriteFile(file1, []byte("content 1"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(file2, []byte("content 2"), 0644)
		require.NoError(t, err)

		// Set old timestamps
		oldTime := time.Now().Add(-1 * time.Hour)
		err = os.Chtimes(file1, oldTime, oldTime)
		require.NoError(t, err)
		err = os.Chtimes(file2, oldTime, oldTime)
		require.NoError(t, err)

		// Track which files were "removed" by our mock function
		removedFiles := make([]string, 0)
		mockRemoveFunc := func(file string) error {
			removedFiles = append(removedFiles, file)
			return nil
		}

		// Clean files before now
		pattern := filepath.Join(tmpDir, "*.txt")
		err = clean(pattern, time.Now(), mockRemoveFunc)
		require.NoError(t, err)

		// Check that both files were passed to remove function
		assert.Len(t, removedFiles, 2, "both files should be passed to remove function")
		assert.Contains(t, removedFiles, file1, "file1 should be in removed files")
		assert.Contains(t, removedFiles, file2, "file2 should be in removed files")

		// Files should still exist since we used mock remove function
		_, err = os.Stat(file1)
		assert.NoError(t, err, "file1 should still exist with mock remove function")
		_, err = os.Stat(file2)
		assert.NoError(t, err, "file2 should still exist with mock remove function")
	})

	t.Run("remove function returns error", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test file
		testFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)

		// Set old timestamp
		oldTime := time.Now().Add(-1 * time.Hour)
		err = os.Chtimes(testFile, oldTime, oldTime)
		require.NoError(t, err)

		// Mock remove function that returns error
		mockRemoveFunc := func(file string) error {
			return assert.AnError
		}

		// Clean should return the error from remove function
		pattern := filepath.Join(tmpDir, "*.txt")
		err = clean(pattern, time.Now(), mockRemoveFunc)
		assert.Error(t, err, "clean should return error when remove function fails")
		assert.Equal(t, assert.AnError, err, "should return the exact error from remove function")
	})

	t.Run("files newer than cutoff are not removed", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test files
		oldFile := filepath.Join(tmpDir, "old.txt")
		newFile := filepath.Join(tmpDir, "new.txt")

		err := os.WriteFile(oldFile, []byte("old content"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(newFile, []byte("new content"), 0644)
		require.NoError(t, err)

		// Set timestamps
		oldTime := time.Now().Add(-2 * time.Hour)
		newTime := time.Now().Add(-30 * time.Minute)

		err = os.Chtimes(oldFile, oldTime, oldTime)
		require.NoError(t, err)
		err = os.Chtimes(newFile, newTime, newTime)
		require.NoError(t, err)

		// Track removed files
		removedFiles := make([]string, 0)
		mockRemoveFunc := func(file string) error {
			removedFiles = append(removedFiles, file)
			return nil
		}

		// Clean files before 1 hour ago
		cutoffTime := time.Now().Add(-1 * time.Hour)
		pattern := filepath.Join(tmpDir, "*.txt")
		err = clean(pattern, cutoffTime, mockRemoveFunc)
		require.NoError(t, err)

		// Only old file should be removed
		assert.Len(t, removedFiles, 1, "only one file should be removed")
		assert.Contains(t, removedFiles, oldFile, "only old file should be removed")
		assert.NotContains(t, removedFiles, newFile, "new file should not be removed")
	})

	t.Run("invalid pattern in clean function", func(t *testing.T) {
		mockRemoveFunc := func(file string) error {
			return nil
		}

		// Use invalid glob pattern
		err := clean("[", time.Now(), mockRemoveFunc)
		assert.Error(t, err, "clean should return error for invalid pattern")
	})

	t.Run("empty pattern matches nothing", func(t *testing.T) {
		removedFiles := make([]string, 0)
		mockRemoveFunc := func(file string) error {
			removedFiles = append(removedFiles, file)
			return nil
		}

		// Empty pattern should match nothing
		err := clean("", time.Now(), mockRemoveFunc)
		assert.NoError(t, err, "clean should not error with empty pattern")
		assert.Len(t, removedFiles, 0, "no files should be removed with empty pattern")
	})
}
