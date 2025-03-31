package host

import (
	"os"
	"runtime"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
)

// ref. https://github.com/google/cadvisor/blob/854445c010e0b634fcd855a20681ae986da235df/machine/info.go#L40
const bootIDPath = "/proc/sys/kernel/random/boot_id"

// Returns an empty string if the boot ID is not found.
func GetBootID() (string, error) {
	return readBootID(bootIDPath)
}

func readBootID(file string) (string, error) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

var currentBootID string

func init() {
	if runtime.GOOS != "linux" {
		return
	}
	var err error
	currentBootID, err = GetBootID()
	if err != nil {
		log.Logger.Errorw("failed to get boot id", "error", err)
	}
}

func CurrentBootID() string {
	return currentBootID
}
