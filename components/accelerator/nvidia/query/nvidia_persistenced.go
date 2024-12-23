package query

import (
	"context"
	"os/exec"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
)

// Returns true if the local machine has "nvidia-persistenced".
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html#usage
func PersistencedExists() bool {
	log.Logger.Debugw("checking if nvidia-persistenced exists")
	_, err := file.LocateExecutable("nvidia-persistenced")
	return err == nil
}

// "pidof nvidia-persistenced"
func PersistencedRunning(ctx context.Context) bool {
	log.Logger.Debugw("checking if nvidia-persistenced is running")
	err := exec.CommandContext(ctx, "pidof", "nvidia-persistenced").Run()
	if err != nil {
		log.Logger.Debugw("failed to check nvidia-persistenced", "error", err)
	}
	return err == nil
}
