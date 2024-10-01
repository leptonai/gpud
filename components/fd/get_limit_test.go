package fd

import (
	"testing"
)

func TestGetLimit(t *testing.T) {
	limit, err := getLimit()
	if err != nil {
		t.Fatalf("failed to get limit: %v", err)
	}
	t.Logf("limit: %v", limit)
}
