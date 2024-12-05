package disk

import (
	"os"
	"testing"
)

func TestGetPartitions(t *testing.T) {
	t.Parallel()

	partitions, err := GetPartitions()
	if err != nil {
		t.Fatalf("failed to get partitions: %v", err)
	}
	yb, err := partitions.YAML()
	if err != nil {
		t.Fatalf("failed to marshal partitions to yaml: %v", err)
	}
	t.Logf("partitions:\n%s\n", string(yb))

	partitions.RenderTable(os.Stdout)
}
