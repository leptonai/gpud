// Package systemd provides the systemd artifacts and variables for the gpud server.
package systemd

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"
)

//go:embed gpud.service
var GPUDService string

const (
	DefaultEnvFile  = "/etc/default/gpud"
	DefaultUnitFile = "/etc/systemd/system/gpud.service"
	DefaultBinPath  = "/usr/sbin/gpud"
)

func DefaultBinExists() bool {
	_, err := os.Stat(DefaultBinPath)
	return err == nil
}

// CreateDefaultEnvFile creates the default environment file for gpud systemd service.
// Assume systemdctl is already installed, and runs on the linux system.
func CreateDefaultEnvFile() error {
	return writeEnvFile(DefaultEnvFile)
}

func writeEnvFile(file string) error {
	if _, err := os.Stat(file); err == nil {
		return addLogFileFlagIfExists(file)
	}

	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(`# gpud environment variables are set here
FLAGS="--log-level=info --log-file=/var/log/gpud.log"
`)
	return err
}

func addLogFileFlagIfExists(file string) error {
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	readFile, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer readFile.Close()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(readFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// only edit the line that contains FLAGS=
		if !strings.Contains(line, "FLAGS=") {
			lines = append(lines, line)
			continue
		}

		// FLAGS already contains --log-file flag
		if strings.Contains(line, "--log-file=") {
			lines = append(lines, line)
			continue
		}

		// remove the trailing " character
		line = strings.TrimSuffix(line, "\"")
		line = fmt.Sprintf("%s --log-file=/var/log/gpud.log\"", line)

		lines = append(lines, line)
	}

	writeFile, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer writeFile.Close()

	_, err = writeFile.WriteString(strings.Join(lines, "\n"))
	return err
}
