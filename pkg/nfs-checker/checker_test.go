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
