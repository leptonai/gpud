// Package systemd provides the systemd artifacts and variables for the gpud server.
package systemd

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"tailscale.com/atomicfile"
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

const defaultEnvFileContent = `# gpud environment variables are set here
FLAGS="--log-level=info --log-file=/var/log/gpud.log"
`

func writeEnvFile(file string) error {
	if _, err := os.Stat(file); err == nil {
		return addLogFileFlagIfExists(file)
	}
	return atomicfile.WriteFile(file, []byte(defaultEnvFileContent), 0644)
}

func addLogFileFlagIfExists(file string) error {
	lines, err := processEnvFileLines(file)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(file, []byte(strings.Join(lines, "\n")), 0644)
}

// processEnvFileLines reads all lines from the environment file and processes each line,
// adding the log-file flag to the FLAGS variable if it doesn't already exist.
func processEnvFileLines(file string) ([]string, error) {
	readFile, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
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

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
