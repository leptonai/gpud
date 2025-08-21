package memory

import (
	"bufio"
	"os"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_processLineBPFJITAllocExec(t *testing.T) {
	f, err := os.Open("testdata/vmallocinfo.bpf_jit_alloc_exec")
	require.NoError(t, err)
	defer f.Close()

	totalSize := uint64(0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		size, err := processLineBPFJITAllocExec(line)
		require.NoError(t, err)
		totalSize += size
	}

	expected := uint64(3977216)
	assert.Equal(t, expected, totalSize)

	t.Logf("totalSize: %s", humanize.IBytes(totalSize))
}

func Test_readBPFJITBufferBytes_Success(t *testing.T) {
	// Test successful read from the test file
	totalSize, err := readBPFJITBufferBytes("testdata/vmallocinfo.bpf_jit_alloc_exec")
	require.NoError(t, err)

	expected := uint64(3977216)
	assert.Equal(t, expected, totalSize)

	t.Logf("totalSize: %s", humanize.IBytes(totalSize))
}

func Test_readBPFJITBufferBytes_FileNotFound(t *testing.T) {
	// Test file not found error
	_, err := readBPFJITBufferBytes("testdata/nonexistent_file")
	assert.Error(t, err)
}

func Test_readBPFJITBufferBytes_EmptyFile(t *testing.T) {
	// Create a temporary empty file
	tmpFile, err := os.CreateTemp("", "empty_vmallocinfo")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Test with empty file
	size, err := readBPFJITBufferBytes(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), size)
}

func Test_readBPFJITBufferBytes_InvalidFormat(t *testing.T) {
	// Create a temporary file with invalid format
	tmpFile, err := os.CreateTemp("", "invalid_vmallocinfo")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write a line with "bpf_jit_alloc_exec" but with invalid format (missing the size field)
	_, err = tmpFile.WriteString("0xffffffffc1032000-0xffffffffc1036000 bpf_jit_alloc_exec+0xe/0x20 pages=3 vmalloc N0=3\n")
	require.NoError(t, err)
	tmpFile.Close()

	// Test with invalid format
	_, err = readBPFJITBufferBytes(tmpFile.Name())
	assert.Error(t, err)
}

func Test_processLineBPFJITAllocExec_NoBPFJIT(t *testing.T) {
	// Test line without "bpf_jit_alloc_exec"
	line := []byte("0xffffffffc1032000-0xffffffffc1036000   16384 other_function+0xe/0x20 pages=3 vmalloc N0=3")
	size, err := processLineBPFJITAllocExec(line)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), size)
}

func Test_processLineBPFJITAllocExec_InvalidSize(t *testing.T) {
	// Test line with invalid size field
	line := []byte("0xffffffffc1032000-0xffffffffc1036000   invalid bpf_jit_alloc_exec+0xe/0x20 pages=3 vmalloc N0=3")
	_, err := processLineBPFJITAllocExec(line)
	assert.Error(t, err)
}
