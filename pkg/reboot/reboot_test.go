package reboot

import (
	"context"
	"testing"
)

func TestReboot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := Reboot(ctx)
	if err != ErrNotRoot {
		t.Errorf("Reboot() expected error %v, got %v", ErrNotRoot, err)
	}
}
