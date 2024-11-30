package bpf

import (
	"bufio"
	"os"
	"testing"

	"github.com/dustin/go-humanize"
)

func Test_processLineJITAllocExec(t *testing.T) {
	f, err := os.Open("testdata/vmallocinfo.bpf_jit_alloc_exec")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	totalSize := uint64(0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		size, err := processLineJITAllocExec(line)
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
