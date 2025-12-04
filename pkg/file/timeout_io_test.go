package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatWithTimeout_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "testfile")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	ctx := context.Background()

	// Test successful stat with normal timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := StatWithTimeout(timeoutCtx, testFile)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "testfile", info.Name())
}

func TestStatWithTimeout_Timeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "testfile")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Use a very short timeout that will expire before the operation completes
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait to ensure the context expires
	time.Sleep(500 * time.Millisecond)

	info, err := StatWithTimeout(ctx, testFile)
	require.Error(t, err)
	require.Nil(t, info)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestStatWithTimeout_ContextCanceled(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "testfile")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Test with context that gets canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	info, err := StatWithTimeout(ctx, testFile)
	require.Error(t, err)
	require.Nil(t, info)
	assert.Equal(t, context.Canceled, err)
}

func TestStatWithTimeout_NonExistentFile(t *testing.T) {
	ctx := context.Background()
	nonExistentFile := "/path/that/does/not/exist"

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := StatWithTimeout(timeoutCtx, nonExistentFile)
	require.Error(t, err)
	require.Nil(t, info)
	assert.True(t, os.IsNotExist(err))
}

func TestMkdirAllWithTimeout_Success(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	testPath := filepath.Join(tmpDir, "test", "nested", "dir")

	// Test successful creation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = MkdirAllWithTimeout(ctx, testPath, 0755)
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(testPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestMkdirAllWithTimeout_Timeout(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Test timeout scenario
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(time.Second)

	err = MkdirAllWithTimeout(ctx, filepath.Join(tmpDir, "timeout", "test"), 0755)
	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestWriteFileWithTimeout_Success(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test content")

	// Test successful write
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = WriteFileWithTimeout(ctx, testFile, testData, 0644)
	require.NoError(t, err)

	// Verify file was written
	data, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestWriteFileWithTimeout_Timeout(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Test timeout scenario
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(500 * time.Millisecond) // Ensure context expires

	err = WriteFileWithTimeout(ctx, filepath.Join(tmpDir, "timeout.txt"), []byte("data"), 0644)
	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestWriteFileWithTimeout_Canceled(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Test canceled context scenario
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = WriteFileWithTimeout(ctx, filepath.Join(tmpDir, "canceled.txt"), []byte("data"), 0644)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestReadFileWithTimeout_Success(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test content")

	// Create test file
	require.NoError(t, os.WriteFile(testFile, testData, 0644))

	// Test successful read
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := ReadFileWithTimeout(ctx, testFile)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestReadFileWithTimeout_FileNotFound(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Test file not found
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = ReadFileWithTimeout(ctx, filepath.Join(tmpDir, "nonexistent.txt"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestReadFileWithTimeout_Timeout(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	// Test timeout scenario
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(500 * time.Millisecond) // Ensure context expires

	_, err = ReadFileWithTimeout(ctx, testFile)
	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestReadFileWithTimeout_Canceled(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	// Test canceled context scenario
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = ReadFileWithTimeout(ctx, testFile)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestAllOperationsWithContextScenarios comprehensively tests all operations
// with both timeout and cancellation scenarios
func TestAllOperationsWithContextScenarios(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tests := []struct {
		name      string
		setupCtx  func() (context.Context, context.CancelFunc)
		expectErr error
	}{
		{
			name: "with timeout",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				time.Sleep(500 * time.Millisecond) // Ensure timeout
				return ctx, cancel
			},
			expectErr: context.DeadlineExceeded,
		},
		{
			name: "with cancellation",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			expectErr: context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("StatWithTimeout", func(t *testing.T) {
				ctx, cancel := tt.setupCtx()
				defer cancel()

				testFile := filepath.Join(tmpDir, "stat_test")
				require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

				_, err := StatWithTimeout(ctx, testFile)
				require.Error(t, err)
				assert.Equal(t, tt.expectErr, err)
			})

			t.Run("MkdirAllWithTimeout", func(t *testing.T) {
				ctx, cancel := tt.setupCtx()
				defer cancel()

				err := MkdirAllWithTimeout(ctx, filepath.Join(tmpDir, "mkdir_test"), 0755)
				require.Error(t, err)
				assert.Equal(t, tt.expectErr, err)
			})

			t.Run("WriteFileWithTimeout", func(t *testing.T) {
				ctx, cancel := tt.setupCtx()
				defer cancel()

				err := WriteFileWithTimeout(ctx, filepath.Join(tmpDir, "write_test"), []byte("data"), 0644)
				require.Error(t, err)
				assert.Equal(t, tt.expectErr, err)
			})

			t.Run("ReadFileWithTimeout", func(t *testing.T) {
				ctx, cancel := tt.setupCtx()
				defer cancel()

				testFile := filepath.Join(tmpDir, "read_test")
				require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

				_, err := ReadFileWithTimeout(ctx, testFile)
				require.Error(t, err)
				assert.Equal(t, tt.expectErr, err)
			})
		})
	}
}
