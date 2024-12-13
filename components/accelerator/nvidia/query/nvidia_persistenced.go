package query

import (
	"os/exec"

	"github.com/leptonai/gpud/pkg/file"
)

// Returns true if the local machine has "nvidia-persistenced".
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html#usage
func PersistencedExists() bool {
	_, err := file.LocateExecutable("nvidia-persistenced")
	return err == nil
}

// "pidof nvidia-persistenced"
func PersistencedRunning() bool {
	err := exec.Command("pidof", "nvidia-persistenced").Run()
	return err == nil
}
