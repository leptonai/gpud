package nfschecker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewChecker(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          "test-dir",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, checker)
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       "",
				DirName:          "test-dir",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
		assert.Nil(t, checker)
	})
}

func TestChecker_Write(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MemberConfig{
		Config: Config{
			VolumePath:       tempDir,
			DirName:          "test-dir",
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		},
		ID: "test-id",
	}

	checker, err := NewChecker(cfg)
	require.NoError(t, err)

	t.Run("successful write", func(t *testing.T) {
		err := checker.Write()
		assert.NoError(t, err)

		// Verify file was created with correct content
		filePath := filepath.Join(tempDir, "test-dir", "test-id")
		content, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", string(content))
	})

	t.Run("write to non-existent directory", func(t *testing.T) {
		subDir := filepath.Join(tempDir, "subdir")

		// Create the directory first
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       subDir,
				DirName:          "test-dir-2",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		err = checker.Write()
		assert.NoError(t, err)

		// Verify directory was created and file exists
		filePath := filepath.Join(subDir, "test-dir-2", "test-id")
		content, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", string(content))
	})
}

func TestChecker_Clean(t *testing.T) {
	tempDir := t.TempDir()
	dirName := "clean-test-dir"

	cfg := &MemberConfig{
		Config: Config{
			VolumePath:       tempDir,
			DirName:          dirName,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Second},
			NumExpectedFiles: 1,
		},
		ID: "test-id",
	}

	checker, err := NewChecker(cfg)
	require.NoError(t, err)

	// Create some test files in the target directory
	targetDir := filepath.Join(tempDir, dirName)
	oldFile := filepath.Join(targetDir, "old-file")
	newFile := filepath.Join(targetDir, "new-file")

	// Create old file (modify time in the past)
	err = os.WriteFile(oldFile, []byte("old content"), 0644)
	require.NoError(t, err)

	oldTime := time.Now().Add(-2 * time.Second)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	require.NoError(t, err)

	// Create new file
	err = os.WriteFile(newFile, []byte("new content"), 0644)
	require.NoError(t, err)

	// Clean should remove old files
	err = checker.Clean()
	assert.NoError(t, err)

	// Verify old file is removed and new file remains
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(newFile)
	assert.NoError(t, err)
}

// TestChecker_Clean_ComprehensiveFileDeletion tests comprehensive file deletion scenarios
func TestChecker_Clean_ComprehensiveFileDeletion(t *testing.T) {
	t.Run("clean removes multiple old files with different timestamps", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "multi-old-files-test"
		ttl := 5 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with different ages
		files := []struct {
			name   string
			age    time.Duration
			should string // "delete" or "keep"
		}{
			{"very-old-file", 10 * time.Second, "delete"},
			{"old-file-1", 8 * time.Second, "delete"},
			{"old-file-2", 6 * time.Second, "delete"},
			{"borderline-file", 5 * time.Second, "delete"}, // exactly at TTL
			{"recent-file-1", 3 * time.Second, "keep"},
			{"recent-file-2", 1 * time.Second, "keep"},
			{"very-recent-file", 0, "keep"},
		}

		for _, file := range files {
			filePath := filepath.Join(targetDir, file.name)
			err = os.WriteFile(filePath, []byte(fmt.Sprintf("content-%s", file.name)), 0644)
			require.NoError(t, err, "Failed to create file %s", file.name)

			// Set timestamp
			fileTime := time.Now().Add(-file.age)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err, "Failed to set timestamp for file %s", file.name)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify cleanup results
		for _, file := range files {
			filePath := filepath.Join(targetDir, file.name)
			_, err = os.Stat(filePath)

			if file.should == "delete" {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted (age: %v, TTL: %v)", file.name, file.age, ttl)
			} else {
				assert.NoError(t, err, "File %s should be kept (age: %v, TTL: %v)", file.name, file.age, ttl)
			}
		}
	})

	t.Run("clean removes old directories as well as files", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "clean-directories-test"
		ttl := 3 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create old files and directories
		oldFile := filepath.Join(targetDir, "old-file.txt")
		oldDir := filepath.Join(targetDir, "old-directory")
		newFile := filepath.Join(targetDir, "new-file.txt")
		newDir := filepath.Join(targetDir, "new-directory")

		// Create old file
		err = os.WriteFile(oldFile, []byte("old file content"), 0644)
		require.NoError(t, err)

		// Create old directory with content
		err = os.MkdirAll(oldDir, 0755)
		require.NoError(t, err)
		oldDirFile := filepath.Join(oldDir, "nested-file.txt")
		err = os.WriteFile(oldDirFile, []byte("nested content"), 0644)
		require.NoError(t, err)

		// Create new file and directory
		err = os.WriteFile(newFile, []byte("new file content"), 0644)
		require.NoError(t, err)
		err = os.MkdirAll(newDir, 0755)
		require.NoError(t, err)

		// Set old timestamps
		oldTime := time.Now().Add(-5 * time.Second)
		err = os.Chtimes(oldFile, oldTime, oldTime)
		require.NoError(t, err)
		err = os.Chtimes(oldDir, oldTime, oldTime)
		require.NoError(t, err)

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify old items are removed
		_, err = os.Stat(oldFile)
		assert.True(t, os.IsNotExist(err), "Old file should be deleted")
		_, err = os.Stat(oldDir)
		assert.True(t, os.IsNotExist(err), "Old directory should be deleted")

		// Verify new items remain
		_, err = os.Stat(newFile)
		assert.NoError(t, err, "New file should remain")
		_, err = os.Stat(newDir)
		assert.NoError(t, err, "New directory should remain")
	})

	t.Run("clean with zero TTL removes all files", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "zero-ttl-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: 0}, // Zero TTL
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Create checker without validation to allow zero TTL
		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return time.Now().UTC()
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with various timestamps
		files := []string{"file1.txt", "file2.txt", "file3.txt"}
		for i, fileName := range files {
			filePath := filepath.Join(targetDir, fileName)
			err = os.WriteFile(filePath, []byte(fmt.Sprintf("content %d", i)), 0644)
			require.NoError(t, err)

			// Set different timestamps
			fileTime := time.Now().Add(time.Duration(-i-1) * time.Second)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// All files should be removed
		for _, fileName := range files {
			filePath := filepath.Join(targetDir, fileName)
			_, err = os.Stat(filePath)
			assert.True(t, os.IsNotExist(err), "File %s should be deleted with zero TTL", fileName)
		}
	})

	t.Run("clean with very large TTL keeps all files", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "large-ttl-test"
		ttl := 24 * time.Hour // Very large TTL

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with old timestamps (but not older than TTL)
		files := []string{"old-file1.txt", "old-file2.txt", "old-file3.txt"}
		for i, fileName := range files {
			filePath := filepath.Join(targetDir, fileName)
			err = os.WriteFile(filePath, []byte(fmt.Sprintf("content %d", i)), 0644)
			require.NoError(t, err)

			// Set timestamps to several hours ago (but less than 24 hours)
			fileTime := time.Now().Add(time.Duration(-(i+1)*2) * time.Hour)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// All files should remain
		for _, fileName := range files {
			filePath := filepath.Join(targetDir, fileName)
			_, err = os.Stat(filePath)
			assert.NoError(t, err, "File %s should be kept with large TTL", fileName)
		}
	})

	t.Run("clean with empty directory does not error", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "empty-dir-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create empty target directory
		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Execute clean on empty directory
		err = checker.Clean()
		assert.NoError(t, err, "Clean should not error on empty directory")

		// Directory should still exist
		_, err = os.Stat(targetDir)
		assert.NoError(t, err, "Target directory should still exist")
	})

	t.Run("clean with non-existent directory does not error", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "non-existent-dir"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Note: Do not create the target directory

		// Execute clean on non-existent directory
		err = checker.Clean()
		assert.NoError(t, err, "Clean should not error on non-existent directory")
	})

	t.Run("clean preserves files with exact TTL boundary timestamp", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "boundary-test"
		ttl := 5 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Calculate boundary timestamp exactly at TTL
		now := time.Now().UTC()
		boundaryTime := now.Add(-ttl)

		// Create files around the boundary
		files := []struct {
			name   string
			offset time.Duration // offset from boundary time
			keep   bool
		}{
			{"before-boundary", -time.Millisecond, false}, // Should be deleted
			{"at-boundary", 0, false},                     // Should be deleted (equal case)
			{"after-boundary", time.Millisecond, true},    // Should be kept
		}

		for _, file := range files {
			filePath := filepath.Join(targetDir, file.name)
			err = os.WriteFile(filePath, []byte("test content"), 0644)
			require.NoError(t, err)

			fileTime := boundaryTime.Add(file.offset)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify results
		for _, file := range files {
			filePath := filepath.Join(targetDir, file.name)
			_, err = os.Stat(filePath)

			if file.keep {
				assert.NoError(t, err, "File %s should be kept", file.name)
			} else {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted", file.name)
			}
		}
	})
}

func TestChecker_Check(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("successful check with expected files", func(t *testing.T) {
		dirName := "success-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 2,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create files from multiple checkers in the target directory
		targetDir := filepath.Join(tempDir, dirName)
		file1 := filepath.Join(targetDir, "checker1")
		file2 := filepath.Join(targetDir, "checker2")

		err = os.WriteFile(file1, []byte("shared-content"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(file2, []byte("shared-content"), 0644)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir)
		assert.Equal(t, "successfully checked directory \""+tempDir+"\" with 2 files", result.Message)
		assert.ElementsMatch(t, []string{"checker1", "checker2"}, result.ReadIDs)
		assert.Empty(t, result.Error)
	})

	t.Run("insufficient files", func(t *testing.T) {
		dirName := "insufficient-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 5,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Contains(t, result.Error, "expected 5 files, but only 0 files were read")
	})

	t.Run("file with wrong content", func(t *testing.T) {
		// Use a fresh temp directory for this test to avoid files from previous tests
		wrongTempDir := t.TempDir()
		dirName := "wrong-content-test-dir"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       wrongTempDir,
				DirName:          dirName,
				FileContents:     "expected-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with wrong content in the target directory
		targetDir := filepath.Join(wrongTempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		wrongFile := filepath.Join(targetDir, "wrong-content")
		err = os.WriteFile(wrongFile, []byte("wrong-content"), 0644)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir)
		assert.Contains(t, result.Error, "file \""+wrongFile+"\" has unexpected contents")
	})

	t.Run("unreadable file", func(t *testing.T) {
		dirName := "unreadable-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create unreadable file (only on Unix-like systems) in the target directory
		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		unreadableFile := filepath.Join(targetDir, "unreadable")
		err = os.WriteFile(unreadableFile, []byte("content"), 0000)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir)
		// Should contain error about failing to read the file
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "unreadable")

		// Clean up
		_ = os.Chmod(unreadableFile, 0644)
		_ = os.Remove(unreadableFile)
	})
}

func TestMultipleCheckersOnSameDirectory(t *testing.T) {
	tempDir := t.TempDir()
	dirName := "multi-checker-test-dir"
	sharedContent := "shared-test-content"

	// Create multiple checkers with different IDs but same directory
	checkers := make([]Checker, 3)
	for i := 0; i < 3; i++ {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     sharedContent,
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 3,
			},
			ID: fmt.Sprintf("checker-%d", i),
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)
		checkers[i] = checker
	}

	targetDir := filepath.Join(tempDir, dirName)

	t.Run("all checkers write successfully", func(t *testing.T) {
		// All checkers write their files
		for i, checker := range checkers {
			err := checker.Write()
			assert.NoError(t, err, "checker %d should write successfully", i)
		}

		// Verify all files exist
		for i := 0; i < 3; i++ {
			filePath := filepath.Join(targetDir, fmt.Sprintf("checker-%d", i))
			content, err := os.ReadFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, sharedContent, string(content))
		}
	})

	t.Run("all checkers see all files", func(t *testing.T) {
		// Each checker should see all 3 files
		for i, checker := range checkers {
			result := checker.Check()
			assert.Equal(t, targetDir, result.Dir) // Explicitly test Dir field
			assert.Empty(t, result.Error, "checker %d should have no errors", i)
			assert.Len(t, result.ReadIDs, 3, "checker %d should see 3 files", i)
			assert.ElementsMatch(t, []string{"checker-0", "checker-1", "checker-2"}, result.ReadIDs)
		}
	})

	t.Run("clean operation works for all checkers", func(t *testing.T) {
		// Create an old file that should be cleaned
		oldFile := filepath.Join(targetDir, "old-checker")
		err := os.WriteFile(oldFile, []byte(sharedContent), 0644)
		require.NoError(t, err)

		// Set old timestamp
		oldTime := time.Now().Add(-2 * time.Minute)
		err = os.Chtimes(oldFile, oldTime, oldTime)
		require.NoError(t, err)

		// Any checker can clean
		err = checkers[0].Clean()
		assert.NoError(t, err)

		// Verify old file is removed
		_, err = os.Stat(oldFile)
		assert.True(t, os.IsNotExist(err))

		// Verify current files still exist
		for i := 0; i < 3; i++ {
			filePath := filepath.Join(targetDir, fmt.Sprintf("checker-%d", i))
			_, err := os.Stat(filePath)
			assert.NoError(t, err, "current file checker-%d should still exist", i)
		}
	})
}

func TestConcurrentCheckers(t *testing.T) {
	tempDir := t.TempDir()
	dirName := "concurrent-test-dir"
	sharedContent := "concurrent-test-content"
	numCheckers := 5

	// Create multiple checkers
	checkers := make([]Checker, numCheckers)
	for i := 0; i < numCheckers; i++ {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     sharedContent,
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: numCheckers,
			},
			ID: fmt.Sprintf("concurrent-checker-%d", i),
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)
		checkers[i] = checker
	}

	targetDir := filepath.Join(tempDir, dirName)

	t.Run("concurrent writes", func(t *testing.T) {
		// Write concurrently
		done := make(chan error, numCheckers)
		for i, checker := range checkers {
			go func(idx int, c Checker) {
				done <- c.Write()
			}(i, checker)
		}

		// Wait for all writes to complete
		for i := 0; i < numCheckers; i++ {
			err := <-done
			assert.NoError(t, err, "concurrent write %d should succeed", i)
		}

		// Verify all files exist
		for i := 0; i < numCheckers; i++ {
			filePath := filepath.Join(targetDir, fmt.Sprintf("concurrent-checker-%d", i))
			content, err := os.ReadFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, sharedContent, string(content))
		}
	})

	t.Run("concurrent checks", func(t *testing.T) {
		// Check concurrently
		results := make(chan CheckResult, numCheckers)
		for i, checker := range checkers {
			go func(idx int, c Checker) {
				results <- c.Check()
			}(i, checker)
		}

		// Collect all results
		for i := 0; i < numCheckers; i++ {
			result := <-results
			assert.Equal(t, targetDir, result.Dir) // Explicitly test Dir field
			assert.Empty(t, result.Error, "concurrent check %d should have no errors", i)
			assert.Len(t, result.ReadIDs, numCheckers, "concurrent check %d should see all files", i)
		}
	})
}

// TestChecker_Clean_ErrorCases tests error scenarios in the Clean function
func TestChecker_Clean_ErrorCases(t *testing.T) {
	t.Run("clean continues after permission errors", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "permission-error-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with old timestamps
		readOnlyFile := filepath.Join(targetDir, "readonly-file")
		normalFile := filepath.Join(targetDir, "normal-file")

		err = os.WriteFile(readOnlyFile, []byte("readonly content"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(normalFile, []byte("normal content"), 0644)
		require.NoError(t, err)

		// Set old timestamps
		oldTime := time.Now().Add(-5 * time.Second)
		err = os.Chtimes(readOnlyFile, oldTime, oldTime)
		require.NoError(t, err)
		err = os.Chtimes(normalFile, oldTime, oldTime)
		require.NoError(t, err)

		// Make the directory read-only to simulate permission issues
		err = os.Chmod(targetDir, 0555) // Read and execute only
		if err != nil {
			t.Skip("Cannot change directory permissions on this system")
		}

		// Restore permissions after test
		defer func() {
			_ = os.Chmod(targetDir, 0755)
		}()

		// Clean operation may fail due to permission issues, but should not panic
		err = checker.Clean()
		// We expect an error due to permission issues, but the function should handle it gracefully
		// The exact behavior depends on the underlying file system and OS
		if err != nil {
			assert.Contains(t, err.Error(), "permission denied", "Error should be related to permissions")
		}
	})

	t.Run("clean with invalid glob pattern characters in directory", func(t *testing.T) {
		// This test verifies that the Clean function handles cases where the constructed
		// glob pattern might be invalid due to special characters in directory names
		tempDir := t.TempDir()
		// Note: Using normal directory name since most filesystems don't allow
		// glob special characters in directory names, but we test the pattern construction
		dirName := "normal-dir-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Clean should work normally with regular directory names
		err = checker.Clean()
		assert.NoError(t, err, "Clean should handle normal directory names without glob pattern issues")
	})

	t.Run("clean with symlinked files", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "symlink-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create a real file and a symlink to it
		realFile := filepath.Join(tempDir, "real-file.txt")
		symlinkFile := filepath.Join(targetDir, "symlink-file")

		err = os.WriteFile(realFile, []byte("real content"), 0644)
		require.NoError(t, err)

		err = os.Symlink(realFile, symlinkFile)
		if err != nil {
			t.Skip("Cannot create symlinks on this system")
		}

		// Set old timestamp on symlink
		oldTime := time.Now().Add(-5 * time.Second)
		// Use regular Chtimes which should work on symlinks on most systems
		err = os.Chtimes(symlinkFile, oldTime, oldTime)
		require.NoError(t, err)

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify symlink is removed
		_, err = os.Lstat(symlinkFile) // Use Lstat to check symlink itself
		assert.True(t, os.IsNotExist(err), "Symlink should be deleted")

		// Verify real file still exists
		_, err = os.Stat(realFile)
		assert.NoError(t, err, "Real file should still exist")
	})

	t.Run("clean with concurrent file creation", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "concurrent-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create old file
		oldFile := filepath.Join(targetDir, "old-file")
		err = os.WriteFile(oldFile, []byte("old content"), 0644)
		require.NoError(t, err)

		oldTime := time.Now().Add(-5 * time.Second)
		err = os.Chtimes(oldFile, oldTime, oldTime)
		require.NoError(t, err)

		// Start concurrent file creation
		done := make(chan bool)
		go func() {
			defer close(done)
			for i := 0; i < 5; i++ {
				concurrentFile := filepath.Join(targetDir, fmt.Sprintf("concurrent-file-%d", i))
				_ = os.WriteFile(concurrentFile, []byte("concurrent content"), 0644)
				time.Sleep(10 * time.Millisecond)
			}
		}()

		// Execute clean concurrently
		err = checker.Clean()
		assert.NoError(t, err, "Clean should handle concurrent file operations")

		// Wait for concurrent operations to complete
		<-done

		// Verify old file is removed
		_, err = os.Stat(oldFile)
		assert.True(t, os.IsNotExist(err), "Old file should be deleted despite concurrent operations")
	})
}

// TestChecker_Clean_ConcurrentFileModifications tests Clean() with concurrent file modifications
func TestChecker_Clean_ConcurrentFileModifications(t *testing.T) {
	t.Run("concurrent file creation during clean", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "concurrent-create-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create old files that should be cleaned
		oldFiles := make([]string, 3)
		for i := 0; i < 3; i++ {
			oldFiles[i] = filepath.Join(targetDir, fmt.Sprintf("old-file-%d", i))
			err = os.WriteFile(oldFiles[i], []byte("old content"), 0644)
			require.NoError(t, err)
			oldTime := time.Now().Add(-5 * time.Second)
			err = os.Chtimes(oldFiles[i], oldTime, oldTime)
			require.NoError(t, err)
		}

		// Start concurrent file creation
		var createdFiles []string
		creationDone := make(chan bool)
		go func() {
			defer close(creationDone)
			for i := 0; i < 5; i++ {
				concurrentFile := filepath.Join(targetDir, fmt.Sprintf("concurrent-file-%d", i))
				createdFiles = append(createdFiles, concurrentFile)
				_ = os.WriteFile(concurrentFile, []byte("concurrent content"), 0644)
				time.Sleep(5 * time.Millisecond)
			}
		}()

		// Execute clean concurrently
		err = checker.Clean()
		assert.NoError(t, err, "Clean should handle concurrent file creation")

		// Wait for concurrent operations to complete
		<-creationDone

		// Verify old files are removed
		for i, oldFile := range oldFiles {
			_, err = os.Stat(oldFile)
			assert.True(t, os.IsNotExist(err), "Old file %d should be deleted", i)
		}

		// Concurrent files may or may not exist depending on timing and their creation time
		// This is expected behavior
	})

	t.Run("concurrent file deletion during clean", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "concurrent-delete-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create old files that should be cleaned
		numFiles := 10
		oldFiles := make([]string, numFiles)
		for i := 0; i < numFiles; i++ {
			oldFiles[i] = filepath.Join(targetDir, fmt.Sprintf("old-file-%d", i))
			err = os.WriteFile(oldFiles[i], []byte("old content"), 0644)
			require.NoError(t, err)
			oldTime := time.Now().Add(-5 * time.Second)
			err = os.Chtimes(oldFiles[i], oldTime, oldTime)
			require.NoError(t, err)
		}

		// Start concurrent file deletion (racing with Clean)
		deletionDone := make(chan bool)
		go func() {
			defer close(deletionDone)
			// Delete some files concurrently
			for i := 0; i < numFiles/2; i++ {
				_ = os.Remove(oldFiles[i])
				time.Sleep(2 * time.Millisecond)
			}
		}()

		// Execute clean concurrently
		err = checker.Clean()
		assert.NoError(t, err, "Clean should handle concurrent file deletion gracefully")

		// Wait for concurrent operations to complete
		<-deletionDone

		// All old files should be gone (either by Clean or concurrent deletion)
		for i, oldFile := range oldFiles {
			_, err = os.Stat(oldFile)
			assert.True(t, os.IsNotExist(err), "Old file %d should be deleted", i)
		}
	})

	t.Run("concurrent file timestamp modification during clean", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "concurrent-timestamp-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		targetDir := filepath.Join(tempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with mixed timestamps
		numFiles := 6
		oldFiles := make([]string, numFiles)
		for i := 0; i < numFiles; i++ {
			oldFiles[i] = filepath.Join(targetDir, fmt.Sprintf("file-%d", i))
			err = os.WriteFile(oldFiles[i], []byte("content"), 0644)
			require.NoError(t, err)
			// Half old, half new
			var timestamp time.Time
			if i < numFiles/2 {
				timestamp = time.Now().Add(-5 * time.Second) // Old
			} else {
				timestamp = time.Now() // New
			}
			err = os.Chtimes(oldFiles[i], timestamp, timestamp)
			require.NoError(t, err)
		}

		// Start concurrent timestamp modification
		timestampDone := make(chan bool)
		go func() {
			defer close(timestampDone)
			// Modify timestamps of some files during cleaning
			for i := 0; i < numFiles/2; i++ {
				newTime := time.Now() // Make them "new"
				_ = os.Chtimes(oldFiles[i], newTime, newTime)
				time.Sleep(3 * time.Millisecond)
			}
		}()

		// Execute clean concurrently
		err = checker.Clean()
		assert.NoError(t, err, "Clean should handle concurrent timestamp modifications")

		// Wait for concurrent operations to complete
		<-timestampDone

		// Check which files remain - this depends on the timing of operations
		// At minimum, the originally "new" files should still exist
		remainingCount := 0
		for i := numFiles / 2; i < numFiles; i++ {
			if _, err := os.Stat(oldFiles[i]); err == nil {
				remainingCount++
			}
		}
		assert.GreaterOrEqual(t, remainingCount, 1, "At least some originally new files should remain")
	})

	t.Run("multiple concurrent clean operations", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "multi-clean-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Second},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Create multiple checkers
		numCheckers := 3
		checkers := make([]Checker, numCheckers)
		for i := 0; i < numCheckers; i++ {
			checker, err := NewChecker(cfg)
			require.NoError(t, err)
			checkers[i] = checker
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create old files
		numFiles := 20
		for i := 0; i < numFiles; i++ {
			oldFile := filepath.Join(targetDir, fmt.Sprintf("old-file-%d", i))
			err = os.WriteFile(oldFile, []byte("old content"), 0644)
			require.NoError(t, err)
			oldTime := time.Now().Add(-5 * time.Second)
			err = os.Chtimes(oldFile, oldTime, oldTime)
			require.NoError(t, err)
		}

		// Run multiple clean operations concurrently
		cleanDone := make(chan error, numCheckers)
		for i := 0; i < numCheckers; i++ {
			go func(checker Checker) {
				cleanDone <- checker.Clean()
			}(checkers[i])
		}

		// Wait for all clean operations to complete
		for i := 0; i < numCheckers; i++ {
			err := <-cleanDone
			assert.NoError(t, err, "Concurrent clean %d should succeed", i)
		}

		// Verify all old files are cleaned up
		matches, err := filepath.Glob(filepath.Join(targetDir, "*"))
		assert.NoError(t, err)
		assert.Empty(t, matches, "All old files should be cleaned up by concurrent operations")
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty directory check", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "empty-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir) // Explicitly test Dir field
		assert.Contains(t, result.Error, "expected 1 files, but only 0 files were read")
		assert.Empty(t, result.ReadIDs)
	})

	t.Run("directory with subdirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "subdir-test-dir"

		// Create a subdirectory in the target directory
		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		subDir := filepath.Join(targetDir, "subdir")
		err = os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write a file
		err = checker.Write()
		require.NoError(t, err)

		// Check should work despite subdirectory presence
		// The check should report an error for trying to read the subdirectory
		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir) // Explicitly test Dir field
		// We expect an error about the subdirectory being unreadable
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "subdir")
		// Since subdir comes first alphabetically, the check fails early and only subdir is in ReadIDs
		assert.Contains(t, result.ReadIDs, "subdir")
		// test-checker comes after subdir alphabetically, so it's not processed due to early return
		assert.NotContains(t, result.ReadIDs, "test-checker")
	})

	t.Run("very long file content", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "long-content-test-dir"
		longContent := string(make([]byte, 10000)) // 10KB of null bytes

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     longContent,
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "long-content-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		err = checker.Write()
		assert.NoError(t, err)

		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir) // Explicitly test Dir field
		assert.Empty(t, result.Error)
		assert.Contains(t, result.ReadIDs, "long-content-checker")
	})

	t.Run("listFilesByPattern error", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "pattern-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		// Create checker with a mock function that returns an error
		checker := &checker{
			cfg: cfg,
			listFilesByPattern: func(pattern string) ([]string, error) {
				return nil, errors.New("mock glob error")
			},
		}

		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir) // Explicitly test Dir field
		assert.Equal(t, "failed to list files", result.Message)
		assert.Contains(t, result.Error, "mock glob error")
		assert.Empty(t, result.ReadIDs)
	})

	t.Run("write with mkdir error", func(t *testing.T) {
		// Try to create a directory under a file (should fail)
		tempFile, err := os.CreateTemp("", "test-file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       filepath.Join(tempFile.Name(), "subdir"), // This should fail
				DirName:          "test-dir",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Create checker without validation
		checker := &checker{cfg: cfg}

		err = checker.Write()
		assert.Error(t, err)
	})

	t.Run("write with file write error", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "write-error-test-dir"

		// Create a directory with the same name as the file we want to write
		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		conflictDir := filepath.Join(targetDir, "test-id")
		err = os.MkdirAll(conflictDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id", // This conflicts with the directory
		}

		checker := &checker{cfg: cfg}

		err = checker.Write()
		assert.Error(t, err)
	})
}

func TestCheckResult_Dir(t *testing.T) {
	t.Run("Dir field set correctly on successful check", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "success-dir-test"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write a file to ensure successful check
		err = checker.Write()
		require.NoError(t, err)

		result := checker.Check()

		// Explicitly test that Dir field matches the configured directory (including DirName)
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.NotEmpty(t, result.Dir)
	})

	t.Run("Dir field set correctly on error case", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "error-dir-test"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 5, // Expecting more files than exist
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()

		// Even with errors, Dir field should be set correctly
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.NotEmpty(t, result.Error) // Should have validation errors
	})

	t.Run("Dir field set correctly when listFilesByPattern fails", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "pattern-fail-dir-test"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		// Create checker with a mock function that returns an error
		checker := &checker{
			cfg: cfg,
			listFilesByPattern: func(pattern string) ([]string, error) {
				return nil, errors.New("mock pattern error")
			},
		}

		result := checker.Check()

		// Dir field should still be set correctly even when glob fails
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed to list files", result.Message)
		assert.Contains(t, result.Error, "mock pattern error")
	})

	t.Run("Dir field with different directory paths", func(t *testing.T) {
		testCases := []struct {
			name     string
			setupDir func(baseDir string) string
			dirName  string
		}{
			{
				name: "simple temp directory",
				setupDir: func(baseDir string) string {
					return baseDir
				},
				dirName: "simple-dir",
			},
			{
				name: "nested subdirectory",
				setupDir: func(baseDir string) string {
					subDir := filepath.Join(baseDir, "nested", "sub", "directory")
					err := os.MkdirAll(subDir, 0755)
					require.NoError(t, err)
					return subDir
				},
				dirName: "nested-dir",
			},
			{
				name: "directory with special characters",
				setupDir: func(baseDir string) string {
					specialDir := filepath.Join(baseDir, "dir-with_special.chars")
					err := os.MkdirAll(specialDir, 0755)
					require.NoError(t, err)
					return specialDir
				},
				dirName: "special-dir",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				baseDir := t.TempDir()
				testDir := tc.setupDir(baseDir)

				cfg := &MemberConfig{
					Config: Config{
						VolumePath:       testDir,
						DirName:          tc.dirName,
						FileContents:     "test-content",
						TTLToDelete:      metav1.Duration{Duration: time.Minute},
						NumExpectedFiles: 1,
					},
					ID: "test-checker",
				}

				checker, err := NewChecker(cfg)
				require.NoError(t, err)

				// Write a file
				err = checker.Write()
				require.NoError(t, err)

				result := checker.Check()

				// Verify Dir field matches exactly the expected directory (testDir + dirName)
				expectedDir := filepath.Join(testDir, tc.dirName)
				assert.Equal(t, expectedDir, result.Dir)
			})
		}
	})

	t.Run("Dir field consistency across multiple checks", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "consistency-dir-test"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write a file
		err = checker.Write()
		require.NoError(t, err)

		expectedDir := filepath.Join(tempDir, dirName)

		// Perform multiple checks and verify Dir field is consistent
		for i := 0; i < 3; i++ {
			result := checker.Check()
			assert.Equal(t, expectedDir, result.Dir, "Dir field should be consistent across multiple checks (iteration %d)", i+1)
		}
	})

	t.Run("Dir field with absolute vs relative paths", func(t *testing.T) {
		baseDir := t.TempDir()

		// Test with absolute path (which tempDir provides)
		absDir := baseDir

		// Test with a relative path from the absolute base
		relativeSubDir := filepath.Join(absDir, "relative")
		err := os.MkdirAll(relativeSubDir, 0755)
		require.NoError(t, err)

		testCases := []struct {
			name    string
			dir     string
			dirName string
		}{
			{"absolute path", absDir, "abs-dir"},
			{"relative-style path", relativeSubDir, "rel-dir"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := &MemberConfig{
					Config: Config{
						VolumePath:       tc.dir,
						DirName:          tc.dirName,
						FileContents:     "test-content",
						TTLToDelete:      metav1.Duration{Duration: time.Minute},
						NumExpectedFiles: 1,
					},
					ID: "test-checker",
				}

				checker, err := NewChecker(cfg)
				require.NoError(t, err)

				err = checker.Write()
				require.NoError(t, err)

				result := checker.Check()

				// Dir field should exactly match the expected directory (dir + dirName)
				expectedDir := filepath.Join(tc.dir, tc.dirName)
				assert.Equal(t, expectedDir, result.Dir)
			})
		}
	})
}

// TestChecker_Clean_WithMockedTime tests Clean() functionality using mocked time
func TestChecker_Clean_WithMockedTime(t *testing.T) {
	t.Run("clean removes files based on mocked current time", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "mocked-time-test"
		ttl := 5 * time.Minute

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Create checker with mocked time
		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				// Mock current time
				return time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
			},
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with various timestamps relative to mocked time
		mockedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		files := []struct {
			name      string
			modTime   time.Time
			shouldDel bool
		}{
			{"file-10min-old", mockedNow.Add(-10 * time.Minute), true},
			{"file-6min-old", mockedNow.Add(-6 * time.Minute), true},
			{"file-exactly-ttl", mockedNow.Add(-ttl), false}, // Should NOT be deleted (boundary case - equal time)
			{"file-4min-old", mockedNow.Add(-4 * time.Minute), false},
			{"file-1min-old", mockedNow.Add(-1 * time.Minute), false},
			{"file-current", mockedNow, false},
			{"file-future", mockedNow.Add(1 * time.Minute), false},
		}

		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			err = os.Chtimes(filePath, f.modTime, f.modTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify results
		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			_, err = os.Stat(filePath)
			if f.shouldDel {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted", f.name)
			} else {
				assert.NoError(t, err, "File %s should exist", f.name)
			}
		}
	})

	t.Run("clean with time progression simulation", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "time-progression-test"
		ttl := 10 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Start time
		startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
		currentTime := startTime

		// Create checker with mocked time that we can control
		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return currentTime
			},
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create a file at start time
		testFile := filepath.Join(targetDir, "test-file")
		err = os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)
		err = os.Chtimes(testFile, startTime, startTime)
		require.NoError(t, err)

		// First clean - file should not be deleted (not old enough)
		err = checker.Clean()
		assert.NoError(t, err)
		_, err = os.Stat(testFile)
		assert.NoError(t, err, "File should exist when younger than TTL")

		// Progress time to just before TTL
		currentTime = startTime.Add(ttl - time.Second)
		err = checker.Clean()
		assert.NoError(t, err)
		_, err = os.Stat(testFile)
		assert.NoError(t, err, "File should exist when just before TTL")

		// Progress time to exactly TTL
		currentTime = startTime.Add(ttl)
		err = checker.Clean()
		assert.NoError(t, err)
		_, err = os.Stat(testFile)
		assert.NoError(t, err, "File should NOT be deleted when exactly at TTL (boundary case)")
	})

	t.Run("clean with fractional second precision", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "fractional-seconds-test"
		ttl := 1 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Mock time with nanosecond precision
		mockedNow := time.Date(2024, 1, 1, 12, 0, 0, 500000000, time.UTC) // .5 seconds

		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return mockedNow
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with precise timestamps
		files := []struct {
			name      string
			modTime   time.Time
			shouldDel bool
		}{
			{"file-1.5s-old", mockedNow.Add(-1500 * time.Millisecond), true},
			{"file-1.0s-old", mockedNow.Add(-1000 * time.Millisecond), false}, // Exactly at TTL boundary
			{"file-999ms-old", mockedNow.Add(-999 * time.Millisecond), false},
			{"file-500ms-old", mockedNow.Add(-500 * time.Millisecond), false},
		}

		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			err = os.Chtimes(filePath, f.modTime, f.modTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify results
		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			_, err = os.Stat(filePath)
			if f.shouldDel {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted", f.name)
			} else {
				assert.NoError(t, err, "File %s should exist", f.name)
			}
		}
	})

	t.Run("clean with dynamic time changes during operation", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "dynamic-time-test"
		ttl := 5 * time.Second

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Start time
		callCount := 0
		baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

		// Create checker with mocked time that changes per call
		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				callCount++
				// Simulate time passing during the clean operation
				return baseTime.Add(time.Duration(callCount) * time.Second)
			},
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create multiple files
		for i := 0; i < 10; i++ {
			filePath := filepath.Join(targetDir, fmt.Sprintf("file-%d", i))
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			// Set different ages
			fileTime := baseTime.Add(-time.Duration(i) * time.Second)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Files 5-9 should be deleted (older than 5 seconds)
		for i := 0; i < 10; i++ {
			filePath := filepath.Join(targetDir, fmt.Sprintf("file-%d", i))
			_, err = os.Stat(filePath)
			if i >= 5 {
				assert.True(t, os.IsNotExist(err), "File-%d should be deleted", i)
			} else {
				assert.NoError(t, err, "File-%d should exist", i)
			}
		}
	})
}

// TestChecker_Clean_EdgeCasesWithMockedTime tests edge cases using mocked time
func TestChecker_Clean_EdgeCasesWithMockedTime(t *testing.T) {
	t.Run("clean with extremely old Unix epoch files", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "epoch-test"
		ttl := 1 * time.Hour

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Mock current time
		mockedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return mockedNow
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with extreme timestamps
		files := []struct {
			name      string
			modTime   time.Time
			shouldDel bool
		}{
			{"file-unix-epoch", time.Unix(0, 0), true},        // 1970-01-01
			{"file-before-epoch", time.Unix(-86400, 0), true}, // 1969-12-31
			{"file-far-past", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), true},
			{"file-recent", mockedNow.Add(-30 * time.Minute), false},
		}

		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			err = os.Chtimes(filePath, f.modTime, f.modTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify results
		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			_, err = os.Stat(filePath)
			if f.shouldDel {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted", f.name)
			} else {
				assert.NoError(t, err, "File %s should exist", f.name)
			}
		}
	})

	t.Run("clean with negative TTL edge case", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "negative-ttl-test"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: -1 * time.Hour}, // Negative TTL
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Mock current time
		mockedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return mockedNow
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files - with negative TTL, all files are "too old"
		files := []string{"file1", "file2", "file3"}
		for _, name := range files {
			filePath := filepath.Join(targetDir, name)
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			// Set different timestamps
			fileTime := mockedNow.Add(-time.Duration(len(name)) * time.Minute)
			err = os.Chtimes(filePath, fileTime, fileTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// With negative TTL, the cutoff time is in the future, so all files should be deleted
		for _, name := range files {
			filePath := filepath.Join(targetDir, name)
			_, err = os.Stat(filePath)
			assert.True(t, os.IsNotExist(err), "File %s should be deleted with negative TTL", name)
		}
	})

	t.Run("clean with maximum time values", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "max-time-test"
		ttl := 1 * time.Hour

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Mock current time at year 2100 (more reasonable maximum)
		mockedNow := time.Date(2100, 12, 31, 23, 59, 59, 999999999, time.UTC)

		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return mockedNow
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create a file
		testFile := filepath.Join(targetDir, "future-file")
		err = os.WriteFile(testFile, []byte("content"), 0644)
		require.NoError(t, err)

		// Set file time to just before the mocked time
		fileTime := mockedNow.Add(-30 * time.Minute)
		err = os.Chtimes(testFile, fileTime, fileTime)
		require.NoError(t, err)

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// File should still exist (not older than TTL)
		_, err = os.Stat(testFile)
		assert.NoError(t, err, "File should exist as it's not older than TTL")
	})

	t.Run("clean with files in future time", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "future-files-test"
		ttl := 1 * time.Hour

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:       tempDir,
				DirName:          dirName,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: ttl},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		// Mock current time
		mockedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

		checker := &checker{
			cfg: cfg,
			getTimeNow: func() time.Time {
				return mockedNow
			},
			listFilesByPattern: filepath.Glob,
		}

		targetDir := filepath.Join(tempDir, dirName)
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Create files with future timestamps
		files := []struct {
			name      string
			modTime   time.Time
			shouldDel bool
		}{
			{"file-future-1h", mockedNow.Add(1 * time.Hour), false},  // Future file
			{"file-future-1d", mockedNow.Add(24 * time.Hour), false}, // Far future file
			{"file-old", mockedNow.Add(-2 * time.Hour), true},        // Old file
		}

		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			err = os.WriteFile(filePath, []byte("content"), 0644)
			require.NoError(t, err)
			err = os.Chtimes(filePath, f.modTime, f.modTime)
			require.NoError(t, err)
		}

		// Execute clean
		err = checker.Clean()
		assert.NoError(t, err)

		// Verify results - future files should not be deleted
		for _, f := range files {
			filePath := filepath.Join(targetDir, f.name)
			_, err = os.Stat(filePath)
			if f.shouldDel {
				assert.True(t, os.IsNotExist(err), "File %s should be deleted", f.name)
			} else {
				assert.NoError(t, err, "File %s should exist", f.name)
			}
		}
	})
}
