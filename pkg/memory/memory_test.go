package memory

import (
	"testing"
)

func TestGetCurrentProcessRSSInBytes(t *testing.T) {
	bytes, err := GetCurrentProcessRSSInBytes()
	if err != nil {
		t.Fatalf("failed to get bytes: %v", err)
	}
	t.Logf("bytes: %v", bytes)
}
