package memory

import (
	"bufio"
	"os"
	"testing"

	"github.com/dustin/go-humanize"
)

func Test_processLineBPFJITAllocExec(t *testing.T) {
	f, err := os.Open("testdata/vmallocinfo.bpf_jit_alloc_exec")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	totalSize := uint64(0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		size, err := processLineBPFJITAllocExec(line)
		if err != nil {
			t.Fatal(err)
		}
		totalSize += size
	}

	expected := uint64(3977216)
	if totalSize != expected {
		t.Fatalf("expected %d, got %d", expected, totalSize)
	}

	t.Logf("totalSize: %s", humanize.Bytes(totalSize))
}

func Test_readBPFJITBufferBytes_Success(t *testing.T) {
	// Test successful read from the test file
	totalSize, err := readBPFJITBufferBytes("testdata/vmallocinfo.bpf_jit_alloc_exec")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := uint64(3977216)
	if totalSize != expected {
		t.Fatalf("expected %d, got %d", expected, totalSize)
	}

	t.Logf("totalSize: %s", humanize.Bytes(totalSize))
}

func Test_readBPFJITBufferBytes_FileNotFound(t *testing.T) {
	// Test file not found error
	_, err := readBPFJITBufferBytes("testdata/nonexistent_file")
	if err == nil {
		t.Fatal("expected file not found error, got nil")
	}
}

func Test_readBPFJITBufferBytes_EmptyFile(t *testing.T) {
	// Create a temporary empty file
	tmpFile, err := os.CreateTemp("", "empty_vmallocinfo")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Test with empty file
	size, err := readBPFJITBufferBytes(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error for empty file, got %v", err)
	}
	if size != 0 {
		t.Fatalf("expected 0 size for empty file, got %d", size)
	}
}

func Test_readBPFJITBufferBytes_InvalidFormat(t *testing.T) {
	// Create a temporary file with invalid format
	tmpFile, err := os.CreateTemp("", "invalid_vmallocinfo")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a line with "bpf_jit_alloc_exec" but with invalid format (missing the size field)
	_, err = tmpFile.WriteString("0xffffffffc1032000-0xffffffffc1036000 bpf_jit_alloc_exec+0xe/0x20 pages=3 vmalloc N0=3\n")
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test with invalid format
	_, err = readBPFJITBufferBytes(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

func Test_processLineBPFJITAllocExec_NoBPFJIT(t *testing.T) {
	// Test line without "bpf_jit_alloc_exec"
	line := []byte("0xffffffffc1032000-0xffffffffc1036000   16384 other_function+0xe/0x20 pages=3 vmalloc N0=3")
	size, err := processLineBPFJITAllocExec(line)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if size != 0 {
		t.Fatalf("expected 0 size for non-BPF line, got %d", size)
	}
}

func Test_processLineBPFJITAllocExec_InvalidSize(t *testing.T) {
	// Test line with invalid size field
	line := []byte("0xffffffffc1032000-0xffffffffc1036000   invalid bpf_jit_alloc_exec+0xe/0x20 pages=3 vmalloc N0=3")
	_, err := processLineBPFJITAllocExec(line)
	if err == nil {
		t.Fatal("expected error for invalid size field, got nil")
	}
}
