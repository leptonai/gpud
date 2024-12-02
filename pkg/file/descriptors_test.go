package file

import (
	"testing"
)

func TestGetLimit(t *testing.T) {
	limit, err := GetLimit()
	if err != nil {
		t.Fatalf("failed to get limit: %v", err)
	}
	t.Logf("limit: %v", limit)
}

func Test_getLimit(t *testing.T) {
	limit, err := getLimit("./testdata/file-max")
	if err != nil {
		t.Fatalf("failed to get limit: %v", err)
	}

	if limit != 1000000 {
		t.Fatalf("limit is not 1000000: %v", limit)
	}
}

func Test_getFileHandles(t *testing.T) {
	allocated, unused, err := getFileHandles("./testdata/file-nr")
	if err != nil {
		t.Fatalf("failed to get file handles: %v", err)
	}
	t.Logf("allocated: %v, unused: %v", allocated, unused)

	if allocated != 1002592 {
		t.Fatalf("allocated is not 1002592: %v", allocated)
	}
	if unused != 0 {
		t.Fatalf("unused is not 0: %v", unused)
	}
}
