package diagnose

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDiagnose(t *testing.T) {
	dir := getDir()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir + ".tar")
	defer os.RemoveAll("summary.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := run(ctx, dir)
	if err != nil {
		t.Log(err)
	}
}
