package os

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLimit(t *testing.T) {
	// Create a temporary file with test data
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "file-max")
	err := os.WriteFile(testPath, []byte("1000000\n"), 0644)
	assert.NoError(t, err)

	// Test valid case
	limit, err := getLimit(testPath)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000000), limit)

	// Test error cases
	_, err = getLimit(filepath.Join(tempDir, "nonexistent-file"))
	assert.Error(t, err)

	// Test invalid content
	invalidPath := filepath.Join(tempDir, "invalid-file-max")
	err = os.WriteFile(invalidPath, []byte("not-a-number\n"), 0644)
	assert.NoError(t, err)
	_, err = getLimit(invalidPath)
	assert.Error(t, err)
}

func TestGetFileHandles(t *testing.T) {
	// Create a temporary file with test data
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "file-nr")
	err := os.WriteFile(testPath, []byte("1000 500 10000\n"), 0644)
	assert.NoError(t, err)

	// Test valid case
	allocated, unused, err := getFileHandles(testPath)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000), allocated)
	assert.Equal(t, uint64(500), unused)

	// Test error cases
	_, _, err = getFileHandles(filepath.Join(tempDir, "nonexistent-file"))
	assert.Error(t, err)

	// Test invalid content - wrong number of fields
	invalidPath1 := filepath.Join(tempDir, "invalid-file-nr-1")
	err = os.WriteFile(invalidPath1, []byte("1000 500\n"), 0644)
	assert.NoError(t, err)
	_, _, err = getFileHandles(invalidPath1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected number of fields")

	// Test invalid content - not numbers
	invalidPath2 := filepath.Join(tempDir, "invalid-file-nr-2")
	err = os.WriteFile(invalidPath2, []byte("a b c\n"), 0644)
	assert.NoError(t, err)
	_, _, err = getFileHandles(invalidPath2)
	assert.Error(t, err)

	// Test invalid content - second field not a number
	invalidPath3 := filepath.Join(tempDir, "invalid-file-nr-3")
	err = os.WriteFile(invalidPath3, []byte("1000 b 10000\n"), 0644)
	assert.NoError(t, err)
	_, _, err = getFileHandles(invalidPath3)
	assert.Error(t, err)
}
