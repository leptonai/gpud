package nfschecker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChecker(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("valid config", func(t *testing.T) {
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      "test-dir",
				FileContents: "test-content",
			},
			ID: "test-id",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
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

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
		assert.Nil(t, checker)
	})
}

func TestChecker_Write(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &MemberConfig{
		Config: Config{
			VolumePath:   tmpDir,
			DirName:      "test-dir",
			FileContents: "test-content",
		},
		ID: "test-id",
	}

	ctx := context.Background()
	checker, err := NewChecker(ctx, cfg)
	require.NoError(t, err)

	t.Run("successful write", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := checker.Write(ctx)
		assert.NoError(t, err)

		// Verify file was created with correct content
		filePath := filepath.Join(tmpDir, "test-dir", "test-id")
		content, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", string(content))
	})

	t.Run("write to non-existent directory", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "subdir")

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

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		err = checker.Write(ctx2)
		assert.NoError(t, err)

		// Verify directory was created and file exists
		filePath := filepath.Join(subDir, "test-dir-2", "test-id")
		content, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "test-content", string(content))
	})

	t.Run("timeout during mkdir operation", func(t *testing.T) {
		// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
		timeoutTempDir, err := os.MkdirTemp("", "test-timeout-mkdir-*")
		require.NoError(t, err)
		// Ensure cleanup happens even if test fails
		defer func() {
			// Force remove all contents to prevent "directory not empty" errors
			os.RemoveAll(timeoutTempDir)
		}()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   timeoutTempDir,
				DirName:      "timeout-mkdir-dir",
				FileContents: "test-content",
			},
			ID: "timeout-mkdir-id",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Use a timeout that will expire immediately to simulate NFS timeout during mkdir
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure context expires

		err = checker.Write(timeoutCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context canceled during mkdir operation", func(t *testing.T) {
		// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
		cancelTempDir, err := os.MkdirTemp("", "test-cancel-mkdir-*")
		require.NoError(t, err)
		// Ensure cleanup happens even if test fails
		defer func() {
			// Force remove all contents to prevent "directory not empty" errors
			os.RemoveAll(cancelTempDir)
		}()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   cancelTempDir,
				DirName:      "cancel-mkdir-dir",
				FileContents: "test-content",
			},
			ID: "cancel-mkdir-id",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Use a canceled context to simulate NFS operation cancellation during mkdir
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = checker.Write(canceledCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("timeout during write operation", func(t *testing.T) {
		// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
		timeoutTempDir, err := os.MkdirTemp("", "test-timeout-write-*")
		require.NoError(t, err)
		// Ensure cleanup happens even if test fails
		defer func() {
			// Force remove all contents to prevent "directory not empty" errors
			os.RemoveAll(timeoutTempDir)
		}()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   timeoutTempDir,
				DirName:      "timeout-write-dir",
				FileContents: "test-content",
			},
			ID: "timeout-write-id",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Pre-create the directory to avoid mkdir timeout, then test write timeout
		targetDir := filepath.Join(timeoutTempDir, "timeout-write-dir")
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Use a timeout that will expire immediately to simulate NFS timeout during write
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure context expires

		err = checker.Write(timeoutCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context canceled during write operation", func(t *testing.T) {
		// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
		cancelTempDir, err := os.MkdirTemp("", "test-cancel-write-*")
		require.NoError(t, err)
		// Ensure cleanup happens even if test fails
		defer func() {
			// Force remove all contents to prevent "directory not empty" errors
			os.RemoveAll(cancelTempDir)
		}()

		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   cancelTempDir,
				DirName:      "cancel-write-dir",
				FileContents: "test-content",
			},
			ID: "cancel-write-id",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Pre-create the directory to avoid mkdir cancellation, then test write cancellation
		targetDir := filepath.Join(cancelTempDir, "cancel-write-dir")
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)

		// Use a canceled context to simulate NFS operation cancellation during write
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = checker.Write(canceledCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("comprehensive timeout scenarios", func(t *testing.T) {
		// Test that covers various timeout and cancellation scenarios comprehensively
		scenarios := []struct {
			name        string
			ctxFunc     func() (context.Context, context.CancelFunc)
			expectError bool
			errorType   error
		}{
			{
				name: "deadline exceeded",
				ctxFunc: func() (context.Context, context.CancelFunc) {
					ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
					time.Sleep(10 * time.Millisecond) // Ensure timeout
					return ctx, cancel
				},
				expectError: true,
				errorType:   context.DeadlineExceeded,
			},
			{
				name: "context canceled",
				ctxFunc: func() (context.Context, context.CancelFunc) {
					ctx, cancel := context.WithCancel(context.Background())
					cancel() // Cancel immediately
					return ctx, cancel
				},
				expectError: true,
				errorType:   context.Canceled,
			},
			{
				name: "successful operation",
				ctxFunc: func() (context.Context, context.CancelFunc) {
					return context.WithTimeout(context.Background(), 5*time.Second)
				},
				expectError: false,
				errorType:   nil,
			},
		}

		for _, scenario := range scenarios {
			t.Run(scenario.name, func(t *testing.T) {
				// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
				scenarioTempDir, err := os.MkdirTemp("", "test-scenario-*")
				require.NoError(t, err)
				// Ensure cleanup happens even if test fails
				defer func() {
					// Force remove all contents to prevent "directory not empty" errors
					os.RemoveAll(scenarioTempDir)
				}()

				cfg := &MemberConfig{
					Config: Config{
						VolumePath:   scenarioTempDir,
						DirName:      "scenario-test-dir",
						FileContents: "scenario-content",
					},
					ID: "scenario-checker",
				}

				ctx := context.Background()
				checker, err := NewChecker(ctx, cfg)
				require.NoError(t, err)

				// Test Write operation
				writeCtx, writeCancel := scenario.ctxFunc()
				defer writeCancel()
				writeErr := checker.Write(writeCtx)

				if scenario.expectError {
					assert.Error(t, writeErr)
					assert.Equal(t, scenario.errorType, writeErr)
					// If write failed due to context, don't proceed with check
					return
				} else {
					assert.NoError(t, writeErr)
				}

				// Test Check operation (only if write succeeded)
				checkCtx, checkCancel := scenario.ctxFunc()
				defer checkCancel()
				result := checker.Check(checkCtx)

				if scenario.expectError {
					assert.Equal(t, "failed", result.Message)
					assert.Contains(t, result.Error, "failed to read file")
					if errors.Is(scenario.errorType, context.DeadlineExceeded) {
						assert.Contains(t, result.Error, "context deadline exceeded")
						assert.True(t, result.TimeoutError, "TimeoutError should be true for context.DeadlineExceeded")
					} else if errors.Is(scenario.errorType, context.Canceled) {
						assert.Contains(t, result.Error, "context canceled")
						assert.False(t, result.TimeoutError, "TimeoutError should be false for context.Canceled")
					}
				} else {
					assert.Empty(t, result.Error)
					assert.Contains(t, result.Message, "correctly read/wrote")
					assert.False(t, result.TimeoutError, "TimeoutError should be false for successful operation")
				}
			})
		}
	})
}

func TestChecker_Clean(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dirName := "clean-test-dir"

	cfg := &MemberConfig{
		Config: Config{
			VolumePath:   tmpDir,
			DirName:      dirName,
			FileContents: "test-content",
		},
		ID: "test-id",
	}

	ctx := context.Background()
	checker, err := NewChecker(ctx, cfg)
	require.NoError(t, err)

	// Write a file using the checker
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	err = checker.Write(ctx2)
	require.NoError(t, err)

	// Verify file exists
	file := filepath.Join(tmpDir, dirName, "test-id")
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
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("successful check with expected files", func(t *testing.T) {
		dirName := "success-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "checker1",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file using the checker
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		err = checker.Write(ctx2)
		require.NoError(t, err)

		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		result := checker.Check(ctx3)
		targetDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, targetDir, result.Dir)
		assert.Equal(t, "correctly read/wrote on \""+tmpDir+"\"", result.Message)
		assert.Empty(t, result.Error)
	})

	t.Run("file not found", func(t *testing.T) {
		dirName := "not-found-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "checker1",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Don't write file, so Check() should fail
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		result := checker.Check(ctx2)
		expectedDir := filepath.Join(tmpDir, dirName)
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

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Create file with wrong content using the checker's file path
		targetDir := filepath.Join(wrongTempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		wrongFile := filepath.Join(targetDir, "checker1")
		err = os.WriteFile(wrongFile, []byte("wrong-content"), 0644)
		require.NoError(t, err)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		result := checker.Check(ctx2)
		assert.Equal(t, targetDir, result.Dir)
		assert.Contains(t, result.Error, "has unexpected contents")
	})

	t.Run("timeout during read operation", func(t *testing.T) {
		dirName := "timeout-read-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "timeout-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file first
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		err = checker.Write(writeCtx)
		require.NoError(t, err)

		// Use a timeout that will expire immediately to simulate NFS timeout
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure context expires

		result := checker.Check(timeoutCtx)
		expectedDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context deadline exceeded")
		assert.True(t, result.TimeoutError, "TimeoutError should be true for context.DeadlineExceeded")
	})

	t.Run("context canceled during read operation", func(t *testing.T) {
		dirName := "canceled-read-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "canceled-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file first
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		err = checker.Write(writeCtx)
		require.NoError(t, err)

		// Use a canceled context to simulate NFS operation cancellation
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result := checker.Check(canceledCtx)
		expectedDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context canceled")
		assert.False(t, result.TimeoutError, "TimeoutError should be false for context.Canceled")
	})
}

// TestChecker_Check_TimeoutErrorField tests the TimeoutError field in CheckResult
func TestChecker_Check_TimeoutErrorField(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("timeout error sets TimeoutError to true", func(t *testing.T) {
		dirName := "timeout-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "timeout-error-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file first
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		err = checker.Write(writeCtx)
		require.NoError(t, err)

		// Use a timeout that will expire immediately to simulate NFS timeout
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure context expires

		result := checker.Check(timeoutCtx)
		expectedDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context deadline exceeded")
		assert.True(t, result.TimeoutError, "TimeoutError should be true for context.DeadlineExceeded")
	})

	t.Run("context canceled does not set TimeoutError", func(t *testing.T) {
		dirName := "canceled-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "canceled-error-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file first
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		err = checker.Write(writeCtx)
		require.NoError(t, err)

		// Use a canceled context to simulate NFS operation cancellation
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result := checker.Check(canceledCtx)
		expectedDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context canceled")
		assert.False(t, result.TimeoutError, "TimeoutError should be false for context.Canceled")
	})

	t.Run("file not found does not set TimeoutError", func(t *testing.T) {
		dirName := "not-found-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "not-found-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Don't write file, so Check() should fail with file not found
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		result := checker.Check(ctx2)
		expectedDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "no such file or directory")
		assert.False(t, result.TimeoutError, "TimeoutError should be false for file not found error")
	})

	t.Run("successful check does not set TimeoutError", func(t *testing.T) {
		dirName := "success-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tmpDir,
				DirName:      dirName,
				FileContents: "test-content",
			},
			ID: "success-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Write the file using the checker
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		err = checker.Write(ctx2)
		require.NoError(t, err)

		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		result := checker.Check(ctx3)
		targetDir := filepath.Join(tmpDir, dirName)
		assert.Equal(t, targetDir, result.Dir)
		assert.Equal(t, "correctly read/wrote on \""+tmpDir+"\"", result.Message)
		assert.Empty(t, result.Error)
		assert.False(t, result.TimeoutError, "TimeoutError should be false for successful operation")
	})

	t.Run("wrong content does not set TimeoutError", func(t *testing.T) {
		// Use a fresh temp directory for this test to avoid files from previous tests
		wrongTempDir := t.TempDir()
		dirName := "wrong-content-error-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   wrongTempDir,
				DirName:      dirName,
				FileContents: "expected-content",
			},
			ID: "wrong-content-checker",
		}

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Create file with wrong content using the checker's file path
		targetDir := filepath.Join(wrongTempDir, dirName)
		err = os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
		wrongFile := filepath.Join(targetDir, "wrong-content-checker")
		err = os.WriteFile(wrongFile, []byte("wrong-content"), 0644)
		require.NoError(t, err)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		result := checker.Check(ctx2)
		assert.Equal(t, targetDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "has unexpected contents")
		assert.False(t, result.TimeoutError, "TimeoutError should be false for content mismatch error")
	})
}

// TestChecker_TimeoutErrorField_ComprehensiveScenarios tests all timeout scenarios for TimeoutError field
func TestChecker_TimeoutErrorField_ComprehensiveScenarios(t *testing.T) {
	scenarios := []struct {
		name             string
		ctxFunc          func() (context.Context, context.CancelFunc)
		expectError      bool
		expectTimeoutErr bool
		errorType        error
		description      string
	}{
		{
			name: "deadline_exceeded_sets_timeout_error_true",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				time.Sleep(10 * time.Millisecond) // Ensure timeout
				return ctx, cancel
			},
			expectError:      true,
			expectTimeoutErr: true,
			errorType:        context.DeadlineExceeded,
			description:      "context.DeadlineExceeded should set TimeoutError to true",
		},
		{
			name: "context_canceled_sets_timeout_error_false",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			expectError:      true,
			expectTimeoutErr: false,
			errorType:        context.Canceled,
			description:      "context.Canceled should set TimeoutError to false",
		},
		{
			name: "successful_operation_timeout_error_false",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			expectError:      false,
			expectTimeoutErr: false,
			errorType:        nil,
			description:      "successful operation should have TimeoutError false",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Use os.MkdirTemp directly instead of t.TempDir() to have full control over cleanup
			tempDir, err := os.MkdirTemp("", "test-timeout-error-*")
			require.NoError(t, err)
			// Ensure cleanup happens even if test fails
			defer func() {
				// Force remove all contents to prevent "directory not empty" errors
				os.RemoveAll(tempDir)
			}()

			cfg := &MemberConfig{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "comprehensive-test-dir",
					FileContents: "comprehensive-content",
				},
				ID: "comprehensive-checker",
			}

			ctx := context.Background()
			checker, err := NewChecker(ctx, cfg)
			require.NoError(t, err)

			// Write the file first for scenarios that need it
			if !scenario.expectError || scenario.errorType != context.DeadlineExceeded {
				writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer writeCancel()
				err = checker.Write(writeCtx)
				require.NoError(t, err)
			}

			// Test with the scenario's context
			testCtx, testCancel := scenario.ctxFunc()
			defer testCancel()
			result := checker.Check(testCtx)

			if scenario.expectError {
				assert.Equal(t, "failed", result.Message, "Message should be 'failed' for %s", scenario.description)
				assert.NotEmpty(t, result.Error, "Error should not be empty for %s", scenario.description)
			} else {
				assert.Contains(t, result.Message, "correctly read/wrote", "Message should indicate success for %s", scenario.description)
				assert.Empty(t, result.Error, "Error should be empty for %s", scenario.description)
			}

			assert.Equal(t, scenario.expectTimeoutErr, result.TimeoutError, "TimeoutError field mismatch for %s", scenario.description)
		})
	}
}
