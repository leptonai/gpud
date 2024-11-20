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
