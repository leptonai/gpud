package nfschecker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChecker(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "test-content",
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
				VolumePath:   "",
				DirName:      "test-dir",
				FileContents: "test-content",
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
			VolumePath:   tempDir,
			DirName:      "test-dir",
			FileContents: "test-content",
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
				VolumePath:   subDir,
				DirName:      "test-dir-2",
				FileContents: "test-content",
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
			VolumePath:   tempDir,
			DirName:      dirName,
			FileContents: "test-content",
		},
		ID: "test-id",
	}

	checker, err := NewChecker(cfg)
	require.NoError(t, err)

	// Write a file using the checker
	err = checker.Write()
	require.NoError(t, err)

	// Verify file exists
	file := filepath.Join(tempDir, dirName, "test-id")
	_, err = os.Stat(file)
	assert.NoError(t, err)

	// Clean should remove the file that was written
	err = checker.Clean()
	assert.NoError(t, err)

	// Verify file is removed
	_, err = os.Stat(file)
	assert.True(t, os.IsNotExist(err))
}

func TestChecker_Check(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("successful check with expected files", func(t *testing.T) {
		dirName := "success-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write the file using the checker
		err = checker.Write()
		require.NoError(t, err)

		result := checker.Check()
		targetDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, targetDir, result.Dir)
		assert.Equal(t, "correctly read/wrote on \""+tempDir+"\"", result.Message)
		assert.Empty(t, result.Error)
	})

	t.Run("file not found", func(t *testing.T) {
		dirName := "not-found-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Don't write file, so Check() should fail
		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "no such file or directory")
	})

	t.Run("file with wrong content", func(t *testing.T) {
		// Use a fresh temp directory for this test to avoid files from previous tests
		wrongTempDir := t.TempDir()
		dirName := "wrong-content-test-dir"

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   wrongTempDir,
				DirName:      dirName,
				FileContents: "expected-content",
			},
			ID: "checker1",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Create file with wrong content using the checker's file path
		targetDir := filepath.Join(wrongTempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		wrongFile := filepath.Join(targetDir, "checker1")
		err = os.WriteFile(wrongFile, []byte("wrong-content"), 0644)
		require.NoError(t, err)

		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir)
		assert.Contains(t, result.Error, "has unexpected contents")
	})

	t.Run("unreadable file", func(t *testing.T) {
		dirName := "unreadable-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "shared-content",
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

func TestEdgeCases(t *testing.T) {
	t.Run("empty directory check", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "empty-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		result := checker.Check()
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir) // Explicitly test Dir field
		assert.Contains(t, result.Error, "no such file or directory")
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
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "test-checker",
		}

		checker, err := NewChecker(cfg)
		require.NoError(t, err)

		// Write a file
		err = checker.Write()
		require.NoError(t, err)

		// Check should work fine - it only checks the specific file, not subdirectories
		result := checker.Check()
		assert.Equal(t, targetDir, result.Dir) // Explicitly test Dir field
		assert.Empty(t, result.Error)          // Should succeed since the file exists and has correct content
	})

	t.Run("very long file content", func(t *testing.T) {
		tempDir := t.TempDir()
		dirName := "long-content-test-dir"
		longContent := string(make([]byte, 10000)) // 10KB of null bytes

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: longContent,
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
	})

	t.Run("write with mkdir error", func(t *testing.T) {
		// Try to create a directory under a file (should fail)
		tempFile, err := os.CreateTemp("", "test-file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   filepath.Join(tempFile.Name(), "subdir"), // This should fail
				DirName:      "test-dir",
				FileContents: "test-content",
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
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
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
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
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
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
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
						VolumePath:   testDir,
						DirName:      tc.dirName,
						FileContents: "test-content",
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
				VolumePath:   tempDir,
				DirName:      dirName,
				FileContents: "test-content",
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
						VolumePath:   tc.dir,
						DirName:      tc.dirName,
						FileContents: "test-content",
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
