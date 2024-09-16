package systemd

import (
	"context"
	"testing"
	"time"
)

func TestGetLatestJournalctlOutput(t *testing.T) {
	if !JournalctlExists() {
		t.Skip("journalctl does not exist")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := GetLatestJournalctlOutput(ctx, "systemd-journald")
	if err != nil {
		t.Skip(err)
	} else {
		t.Logf("output:\n%s\n", output)
	}
}
