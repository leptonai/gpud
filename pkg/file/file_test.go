package file

import (
	"testing"
)

func TestLocateExecutable(t *testing.T) {
	execPath, err := LocateExecutable("ls")
	if err != nil {
		t.Fatalf("LocateExecutable() failed: %v", err)
	}
	t.Logf("found executable %q", execPath)
}
