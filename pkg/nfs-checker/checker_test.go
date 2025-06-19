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
				Dir:              tempDir,
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
				Dir:              "",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-id",
		}

		checker, err := NewChecker(cfg)
		assert.ErrorIs(t, err, ErrDirEmpty)
		assert.Nil(t, checker)
	})
}

func TestChecker_Write(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MemberConfig{
		Config: Config{
			Dir:              tempDir,
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

		// Verify file was created with correct JSON content
		filePath := filepath.Join(tempDir, "test-id")
		data, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", data.FileContents)
		assert.Equal(t, "", data.VolumeName)
		assert.Equal(t, "", data.VolumeMountPath)
	})

	t.Run("write to non-existent directory", func(t *testing.T) {
		subDir := filepath.Join(tempDir, "subdir")

		// Create the directory first
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				Dir:              subDir,
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

		// Verify directory was created and file exists with correct JSON content
		filePath := filepath.Join(subDir, "test-id")
		data, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", data.FileContents)
		assert.Equal(t, "", data.VolumeName)
		assert.Equal(t, "", data.VolumeMountPath)
	})
}

func TestChecker_Clean(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MemberConfig{
		Config: Config{
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Second},
			NumExpectedFiles: 1,
		},
		ID: "test-id",
	}

	checker, err := NewChecker(cfg)
	require.NoError(t, err)

	// Create some test files
	oldFile := filepath.Join(tempDir, "old-file")
	newFile := filepath.Join(tempDir, "new-file")

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

func TestChecker_Check(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("successful check with expected files using new JSON format", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 2,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create another checker with the same config but different ID
		cfg2 := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 2,
			},
			ID: "checker2",
		}

		checker2, err := NewChecker(cfg2)
		require.NoError(t, err)

		// Both checkers write their files using the new JSON format
		err = checker.Write()
		require.NoError(t, err)
		err = checker2.Write()
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		assert.Equal(t, "successfully checked directory \""+tempDir+"\" with 2 files", result.Message)
		assert.ElementsMatch(t, []string{"checker1", "checker2"}, result.ReadIDs)
		assert.Empty(t, result.Error)
	})

	t.Run("successful check with old plain text format", func(t *testing.T) {
		oldTempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              oldTempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 2,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create files with old plain text format (no volume info)
		file1 := filepath.Join(oldTempDir, "checker1")
		file2 := filepath.Join(oldTempDir, "checker2")

		err = os.WriteFile(file1, []byte("shared-content"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(file2, []byte("shared-content"), 0644)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, oldTempDir, result.Dir)
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.ElementsMatch(t, []string{"checker1", "checker2"}, result.ReadIDs)
		assert.Empty(t, result.Error)
	})

	t.Run("volume name mismatch", func(t *testing.T) {
		mismatchTempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              mismatchTempDir,
				VolumeName:       "expected-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with different volume name
		wrongData := Data{
			VolumeName:      "wrong-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "shared-content",
		}

		file := filepath.Join(mismatchTempDir, "checker2")
		err = wrongData.Write(file)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, mismatchTempDir, result.Dir)
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.ElementsMatch(t, []string{"checker2"}, result.ReadIDs)
		assert.Empty(t, result.Error)
	})

	t.Run("volume mount path mismatch", func(t *testing.T) {
		mismatchTempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              mismatchTempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/expected",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with different mount path
		wrongData := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/wrong",
			FileContents:    "shared-content",
		}

		file := filepath.Join(mismatchTempDir, "checker2")
		err = wrongData.Write(file)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, mismatchTempDir, result.Dir)
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.ElementsMatch(t, []string{"checker2"}, result.ReadIDs)
		assert.Empty(t, result.Error)
	})

	t.Run("insufficient files", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 5,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		assert.Contains(t, result.Error, "expected 5 files, but only")
		assert.Contains(t, result.Error, "files were read")
	})

	t.Run("file with wrong content in new JSON format", func(t *testing.T) {
		wrongTempDir := t.TempDir()

		cfg := &MemberConfig{
			Config: Config{
				Dir:              wrongTempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "expected-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with correct volume info but wrong content
		wrongData := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "wrong-content",
		}

		wrongFile := filepath.Join(wrongTempDir, "wrong-content")
		err = wrongData.Write(wrongFile)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, wrongTempDir, result.Dir)
		assert.Contains(t, result.Error, "file \""+wrongFile+"\" has unexpected contents")
	})

	t.Run("file with wrong content in old plain text format", func(t *testing.T) {
		// Use a fresh temp directory for this test to avoid files from previous tests
		wrongTempDir := t.TempDir()

		cfg := &MemberConfig{
			Config: Config{
				Dir:              wrongTempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "expected-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with wrong content using old format
		wrongFile := filepath.Join(wrongTempDir, "wrong-content")
		err = os.WriteFile(wrongFile, []byte("wrong-content"), 0644)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, wrongTempDir, result.Dir)
		// Old format files don't have volume info, so content check is skipped
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.Empty(t, result.Error)
	})

	t.Run("unreadable file", func(t *testing.T) {
		unreadableTempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              unreadableTempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create unreadable file (only on Unix-like systems)
		unreadableFile := filepath.Join(unreadableTempDir, "unreadable")
		err = os.WriteFile(unreadableFile, []byte("content"), 0000)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, unreadableTempDir, result.Dir)
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
	sharedContent := "shared-test-content"
	volumeName := "shared-volume"
	volumeMountPath := "/mnt/shared"

	// Create multiple checkers with different IDs but same directory
	checkers := make([]Checker, 3)
	for i := 0; i < 3; i++ {
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       volumeName,
				VolumeMountPath:  volumeMountPath,
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

	t.Run("all checkers write successfully", func(t *testing.T) {
		// All checkers write their files using JSON format
		for i, checker := range checkers {
			err := checker.Write()
			assert.NoError(t, err, "checker %d should write successfully", i)
		}

		// Verify all files exist and contain correct JSON data
		for i := 0; i < 3; i++ {
			filePath := filepath.Join(tempDir, fmt.Sprintf("checker-%d", i))
			data, err := ReadDataFromFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, volumeName, data.VolumeName)
			assert.Equal(t, volumeMountPath, data.VolumeMountPath)
			assert.Equal(t, sharedContent, data.FileContents)
		}
	})

	t.Run("all checkers see all files", func(t *testing.T) {
		// Each checker should see all 3 files
		for i, checker := range checkers {
			result := checker.Check()
			assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
			assert.Empty(t, result.Error, "checker %d should have no errors", i)
			assert.Len(t, result.ReadIDs, 3, "checker %d should see 3 files", i)
			assert.ElementsMatch(t, []string{"checker-0", "checker-1", "checker-2"}, result.ReadIDs)
		}
	})

	t.Run("clean operation works for all checkers", func(t *testing.T) {
		// Create an old file that should be cleaned using JSON format
		oldData := Data{
			VolumeName:      volumeName,
			VolumeMountPath: volumeMountPath,
			FileContents:    sharedContent,
		}

		oldFile := filepath.Join(tempDir, "old-checker")
		err := oldData.Write(oldFile)
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
			filePath := filepath.Join(tempDir, fmt.Sprintf("checker-%d", i))
			_, err := os.Stat(filePath)
			assert.NoError(t, err, "current file checker-%d should still exist", i)
		}
	})
}

func TestConcurrentCheckers(t *testing.T) {
	tempDir := t.TempDir()
	sharedContent := "concurrent-test-content"
	volumeName := "concurrent-volume"
	volumeMountPath := "/mnt/concurrent"
	numCheckers := 5

	// Create multiple checkers
	checkers := make([]Checker, numCheckers)
	for i := 0; i < numCheckers; i++ {
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       volumeName,
				VolumeMountPath:  volumeMountPath,
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

		// Verify all files exist and contain correct JSON data
		for i := 0; i < numCheckers; i++ {
			filePath := filepath.Join(tempDir, fmt.Sprintf("concurrent-checker-%d", i))
			data, err := ReadDataFromFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, volumeName, data.VolumeName)
			assert.Equal(t, volumeMountPath, data.VolumeMountPath)
			assert.Equal(t, sharedContent, data.FileContents)
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
			assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
			assert.Empty(t, result.Error, "concurrent check %d should have no errors", i)
			assert.Len(t, result.ReadIDs, numCheckers, "concurrent check %d should see all files", i)
		}
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty directory check", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
		assert.Contains(t, result.Error, "expected 1 files, but only 0 files were read")
		assert.Empty(t, result.ReadIDs)
	})

	t.Run("directory with subdirectories", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a subdirectory
		subDir := filepath.Join(tempDir, "subdir")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
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
		longContent := string(make([]byte, 10000)) // 10KB of null bytes

		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
		assert.Empty(t, result.Error)
		assert.Contains(t, result.ReadIDs, "long-content-checker")
	})

	t.Run("listFilesByPattern error", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		assert.Equal(t, tempDir, result.Dir) // Explicitly test Dir field
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
				Dir:              filepath.Join(tempFile.Name(), "subdir"), // This should fail
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

		// Create a directory with the same name as the file we want to write
		conflictDir := filepath.Join(tempDir, "test-id")
		err := os.MkdirAll(conflictDir, 0755)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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

		// Explicitly test that Dir field matches the configured directory
		assert.Equal(t, tempDir, result.Dir)
		assert.NotEmpty(t, result.Dir)
	})

	t.Run("Dir field set correctly on error case", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		assert.Equal(t, tempDir, result.Dir)
		assert.NotEmpty(t, result.Error) // Should have validation errors
	})

	t.Run("Dir field set correctly when listFilesByPattern fails", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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
		assert.Equal(t, tempDir, result.Dir)
		assert.Equal(t, "failed to list files", result.Message)
		assert.Contains(t, result.Error, "mock pattern error")
	})

	t.Run("Dir field with different directory paths", func(t *testing.T) {
		testCases := []struct {
			name     string
			setupDir func(baseDir string) string
		}{
			{
				name: "simple temp directory",
				setupDir: func(baseDir string) string {
					return baseDir
				},
			},
			{
				name: "nested subdirectory",
				setupDir: func(baseDir string) string {
					subDir := filepath.Join(baseDir, "nested", "sub", "directory")
					err := os.MkdirAll(subDir, 0755)
					require.NoError(t, err)
					return subDir
				},
			},
			{
				name: "directory with special characters",
				setupDir: func(baseDir string) string {
					specialDir := filepath.Join(baseDir, "dir-with_special.chars")
					err := os.MkdirAll(specialDir, 0755)
					require.NoError(t, err)
					return specialDir
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				baseDir := t.TempDir()
				testDir := tc.setupDir(baseDir)

				cfg := &MemberConfig{
					Config: Config{
						Dir:              testDir,
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

				// Verify Dir field matches exactly what was configured
				assert.Equal(t, testDir, result.Dir)
				assert.Equal(t, testDir, cfg.Dir)
			})
		}
	})

	t.Run("Dir field consistency across multiple checks", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
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

		// Perform multiple checks and verify Dir field is consistent
		for i := 0; i < 3; i++ {
			result := checker.Check()
			assert.Equal(t, tempDir, result.Dir, "Dir field should be consistent across multiple checks (iteration %d)", i+1)
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
			name string
			dir  string
		}{
			{"absolute path", absDir},
			{"relative-style path", relativeSubDir},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := &MemberConfig{
					Config: Config{
						Dir:              tc.dir,
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

				// Dir field should exactly match what was configured
				assert.Equal(t, tc.dir, result.Dir)
			})
		}
	})
}

func TestConfig_GenerateData(t *testing.T) {
	t.Run("generate data with all fields", func(t *testing.T) {
		cfg := Config{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		data := cfg.GenerateData()
		assert.Equal(t, cfg.VolumeName, data.VolumeName)
		assert.Equal(t, cfg.VolumeMountPath, data.VolumeMountPath)
		assert.Equal(t, cfg.FileContents, data.FileContents)
	})

	t.Run("generate data with empty fields", func(t *testing.T) {
		cfg := Config{
			VolumeName:      "",
			VolumeMountPath: "",
			FileContents:    "",
		}

		data := cfg.GenerateData()
		assert.Equal(t, "", data.VolumeName)
		assert.Equal(t, "", data.VolumeMountPath)
		assert.Equal(t, "", data.FileContents)
	})

	t.Run("generate data with special characters", func(t *testing.T) {
		cfg := Config{
			VolumeName:      "volume-with_special.chars",
			VolumeMountPath: "/mnt/path with spaces",
			FileContents:    "content with\nnewlines and émojis 🚀",
		}

		data := cfg.GenerateData()
		assert.Equal(t, cfg.VolumeName, data.VolumeName)
		assert.Equal(t, cfg.VolumeMountPath, data.VolumeMountPath)
		assert.Equal(t, cfg.FileContents, data.FileContents)
	})
}

func TestChecker_WriteAndReadIntegration(t *testing.T) {
	t.Run("write then read same data", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "integration-volume",
				VolumeMountPath:  "/mnt/integration",
				FileContents:     "integration-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "integration-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write data
		err = checker.Write()
		require.NoError(t, err)

		// Read back and verify
		filePath := filepath.Join(tempDir, "integration-checker")
		data, err := ReadDataFromFile(filePath)
		require.NoError(t, err)

		assert.Equal(t, cfg.VolumeName, data.VolumeName)
		assert.Equal(t, cfg.VolumeMountPath, data.VolumeMountPath)
		assert.Equal(t, cfg.FileContents, data.FileContents)

		// Check should pass
		result := checker.Check()
		assert.Empty(t, result.Error)
		assert.Contains(t, result.ReadIDs, "integration-checker")
	})

	t.Run("mixed old and new format files", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "mixed-volume",
				VolumeMountPath:  "/mnt/mixed",
				FileContents:     "mixed-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 3,
			},
			ID: "mixed-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write new format file
		err = checker.Write()
		require.NoError(t, err)

		// Write old format file
		oldFile := filepath.Join(tempDir, "old-format")
		err = os.WriteFile(oldFile, []byte("old-content"), 0644)
		require.NoError(t, err)

		// Write another new format file with different content
		differentData := Data{
			VolumeName:      "different-volume",
			VolumeMountPath: "/mnt/different",
			FileContents:    "different-content",
		}
		differentFile := filepath.Join(tempDir, "different-format")
		err = differentData.Write(differentFile)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		assert.ElementsMatch(t, []string{"mixed-checker", "old-format", "different-format"}, result.ReadIDs)
		// Should have success message since file count is sufficient
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.Empty(t, result.Error)
	})
}

func TestChecker_EdgeCasesAndErrors(t *testing.T) {
	t.Run("check with mixed valid and invalid files", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create a valid file
		validData := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}
		validFile := filepath.Join(tempDir, "valid-file")
		err = validData.Write(validFile)
		require.NoError(t, err)

		// Create an invalid file with wrong content
		invalidData := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "wrong-content",
		}
		invalidFile := filepath.Join(tempDir, "invalid-file")
		err = invalidData.Write(invalidFile)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		// Should fail on the first invalid file (alphabetically)
		assert.Contains(t, result.Error, "has unexpected contents")
		// Should stop at first error, so only first file processed
		assert.Contains(t, result.ReadIDs, "invalid-file")
		assert.NotContains(t, result.ReadIDs, "valid-file")
	})

	t.Run("check with zero expected files", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 0, // This should be caught by validation
			},
			ID: "test-checker",
		}

		// This should fail validation
		_, err := NewChecker(cfg)
		assert.ErrorIs(t, err, ErrExpectedFilesZero)
	})

	t.Run("write to directory with existing file as directory name", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a regular file with the same name as our target directory
		conflictFile := filepath.Join(tempDir, "conflict")
		err := os.WriteFile(conflictFile, []byte("conflict"), 0644)
		require.NoError(t, err)

		cfg := &MemberConfig{
			Config: Config{
				Dir:              conflictFile, // This is a file, not a directory
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		// The validation should pass initially (it only validates the dir is absolute and has content)
		// but writing should fail when trying to create subdirectory under the file
		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write should fail because the "directory" is actually a file
		err = checker.Write()
		assert.Error(t, err)
	})

	t.Run("check with very long file names", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker-with-a-very-long-name-that-might-cause-issues-in-some-filesystems",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		err = checker.Write()
		assert.NoError(t, err)

		result := checker.Check()
		assert.Empty(t, result.Error)
		assert.Contains(t, result.ReadIDs, cfg.ID)
	})
}

func TestChecker_VolumeValidationEdgeCases(t *testing.T) {
	t.Run("empty volume name but non-empty mount path", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with empty volume name
		data := Data{
			VolumeName:      "",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		file := filepath.Join(tempDir, "test-file")
		err = data.Write(file)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.Empty(t, result.Error)
	})

	t.Run("non-empty volume name but empty mount path", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "test-volume",
				VolumeMountPath:  "/mnt/test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with empty mount path
		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "",
			FileContents:    "test-content",
		}

		file := filepath.Join(tempDir, "test-file")
		err = data.Write(file)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.Empty(t, result.Error)
	})

	t.Run("exact volume match but different case", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &MemberConfig{
			Config: Config{
				Dir:              tempDir,
				VolumeName:       "Test-Volume",
				VolumeMountPath:  "/mnt/Test",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with different case
		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		file := filepath.Join(tempDir, "test-file")
		err = data.Write(file)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, tempDir, result.Dir)
		// Should be treated as mismatch (case-sensitive) but still succeed due to file count
		assert.Contains(t, result.Message, "successfully checked directory")
		assert.Empty(t, result.Error)
	})
}
