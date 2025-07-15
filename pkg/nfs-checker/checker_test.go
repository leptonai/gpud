package nfschecker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	tempDir := t.TempDir()

	cfg := &MemberConfig{
		Config: Config{
			VolumePath:   tempDir,
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
		timeoutTempDir := t.TempDir()

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
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("context canceled during mkdir operation", func(t *testing.T) {
		cancelTempDir := t.TempDir()

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
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("timeout during write operation", func(t *testing.T) {
		timeoutTempDir := t.TempDir()

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
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("context canceled during write operation", func(t *testing.T) {
		cancelTempDir := t.TempDir()

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
		assert.Equal(t, context.Canceled, err)
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
				scenarioTempDir := t.TempDir()
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
					if scenario.errorType == context.DeadlineExceeded {
						assert.Contains(t, result.Error, "context deadline exceeded")
					} else if scenario.errorType == context.Canceled {
						assert.Contains(t, result.Error, "context canceled")
					}
				} else {
					assert.Empty(t, result.Error)
					assert.Contains(t, result.Message, "correctly read/wrote")
				}
			})
		}
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

	ctx := context.Background()
	checker, err := NewChecker(ctx, cfg)
	require.NoError(t, err)

	// Write a file using the checker
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	err = checker.Write(ctx2)
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

		ctx := context.Background()
		checker, err := NewChecker(ctx, cfg)
		require.NoError(t, err)

		// Don't write file, so Check() should fail
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		result := checker.Check(ctx2)
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
				VolumePath:   tempDir,
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
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context deadline exceeded")
	})

	t.Run("context canceled during read operation", func(t *testing.T) {
		dirName := "canceled-read-test-dir"
		cfg := &MemberConfig{
			Config: Config{
				VolumePath:   tempDir,
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
		expectedDir := filepath.Join(tempDir, dirName)
		assert.Equal(t, expectedDir, result.Dir)
		assert.Equal(t, "failed", result.Message)
		assert.Contains(t, result.Error, "failed to read file")
		assert.Contains(t, result.Error, "context canceled")
	})
}
