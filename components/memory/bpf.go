package memory

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
)

// Fetches the current BPF JIT buffer size in bytes.
// ref. https://github.com/deckhouse/deckhouse/issues/7402
func getCurrentBPFJITBufferBytes(ctx context.Context) (uint64, error) {
	// e.g.,
	// cat /proc/vmallocinfo | grep bpf_jit | awk '{s+=$2} END {print s}'
	if _, err := os.Stat("/proc/vmallocinfo"); err != nil {
		return 0, err
	}

	f, err := os.Open("/proc/vmallocinfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	totalSize := uint64(0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		size, err := processLineBPFJITAllocExec(line)
		if err != nil {
			return 0, err
		}
		totalSize += size
	}

	return totalSize, nil
}

// e.g.,
// 0xffffffffc1032000-0xffffffffc1036000   16384 bpf_jit_alloc_exec+0xe/0x20 pages=3 vmalloc N0=3
//
// e.g.,
// cat /proc/vmallocinfo | grep bpf_jit | awk '{s+=$2} END {print s}'
func processLineBPFJITAllocExec(line []byte) (uint64, error) {
	if !bytes.Contains(line, []byte("bpf_jit_alloc_exec")) {
		return 0, nil
	}

	// split line by whitespace
	fields := bytes.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid line: %q (expected at least 2 fields)", line)
	}

	// Parse the size field (second column)
	size, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0, err
	}
	return size, nil
}
