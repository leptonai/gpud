package host

import (
	"os"
	"strings"
)

// ref. https://github.com/google/cadvisor/blob/854445c010e0b634fcd855a20681ae986da235df/machine/info.go#L40
const bootIDPath = "/proc/sys/kernel/random/boot_id"

// Returns an empty string if the boot ID is not found.
func GetBootID() (string, error) {
	if _, err := os.Stat(bootIDPath); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
