package query

import (
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
