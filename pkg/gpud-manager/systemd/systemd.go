// Package systemd provides the systemd artifacts and variables for the gpud server.
package systemd

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"tailscale.com/atomicfile"
)

//go:embed gpud.service
var gpudService string

func GPUdServiceUnitFileContents() string {
	_, err := os.Stat(DefaultBinPath)
	if errors.Is(err, os.ErrNotExist) {
		// fallback to the old GPUd binary path
		// until this machines its bin path
		gpudService = strings.ReplaceAll(gpudService, DefaultBinPath, DeprecatedDefaultBinPathSbin)
	}
	return gpudService
}

const (
	DefaultEnvFile  = "/etc/default/gpud"
	DefaultUnitFile = "/etc/systemd/system/gpud.service"

	DeprecatedDefaultBinPathSbin = "/usr/sbin/gpud"
	DefaultBinPath               = "/usr/local/bin/gpud"
)

func DefaultBinExists() bool {
	_, err := os.Stat(DefaultBinPath)
	if errors.Is(err, os.ErrNotExist) {
		// fallback to the old GPUd binary path
		// until this machines its bin path
		_, err = os.Stat(DeprecatedDefaultBinPathSbin)
	}
	return err == nil
}

// CreateDefaultEnvFile creates the default environment file for gpud systemd service.
// Assume systemdctl is already installed, and runs on the linux system.
//
// Note: Session credentials (token, machine ID) are NOT passed via environment file.
// login.Login() always writes credentials to the persistent state file (via dataDir).
// When --db-in-memory is enabled, gpud run reads credentials from the persistent file
// and passes them to the server to seed into the in-memory database.
func CreateDefaultEnvFile(endpoint string, dataDir string, dbInMemory bool, sessionProtocol ...string) error {
	return writeEnvFile(DefaultEnvFile, endpoint, dataDir, dbInMemory, sessionProtocol...)
}

func createDefaultEnvFileContent(endpoint string, dataDir string, dbInMemory bool, sessionProtocol ...string) string {
	flags := "--log-level=info --log-file=/var/log/gpud.log"
	if endpoint != "" {
		flags += fmt.Sprintf(" --endpoint=%s", endpoint)
	}
	if dataDir != "" {
		flags += fmt.Sprintf(" --data-dir=%s", dataDir)
	}
	if dbInMemory {
		flags += " --db-in-memory"
	}
	protocol := "auto"
	if len(sessionProtocol) > 0 && sessionProtocol[0] != "" {
		protocol = sessionProtocol[0]
	}
	flags += fmt.Sprintf(" --session-protocol=%s", protocol)

	return fmt.Sprintf(`# gpud environment variables are set here
FLAGS="%s"
`, flags)
}

func writeEnvFile(file string, endpoint string, dataDir string, dbInMemory bool, sessionProtocol ...string) error {
	protocol, err := validateSessionProtocol(sessionProtocol)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(file, []byte(createDefaultEnvFileContent(endpoint, dataDir, dbInMemory, protocol)), 0644)
}

func validateSessionProtocol(values []string) (string, error) {
	if len(values) > 1 {
		return "", fmt.Errorf("expected at most one session protocol, got %d", len(values))
	}
	protocol := "auto"
	if len(values) == 1 && values[0] != "" {
		protocol = values[0]
	}
	switch protocol {
	case "v1", "v2", "auto":
		return protocol, nil
	default:
		return "", fmt.Errorf("unsupported session protocol %q", protocol)
	}
}

func updateFlagsFromExistingEnvFile(file string, endpoint string) error {
	lines, err := processEnvFileLines(file, endpoint)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(file, []byte(strings.Join(lines, "\n")), 0644)
}

// processEnvFileLines reads all lines from the environment file and processes each line,
// adding the log-file flag to the FLAGS variable if it doesn't already exist.
func processEnvFileLines(file string, endpoint string) ([]string, error) {
	readFile, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = readFile.Close()
	}()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(readFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// only edit the line that contains FLAGS=
		if !strings.Contains(line, "FLAGS=") {
			lines = append(lines, line)
			continue
		}

		// FLAGS already contains --log-file flag and --endpoint flag
		if strings.Contains(line, "--log-file=") && (endpoint != "" && strings.Contains(line, "--endpoint=")) {
			lines = append(lines, line)
			continue
		}

		// remove the trailing " character
		line = strings.TrimSuffix(line, "\"")

		if !strings.Contains(line, "--log-file=") {
			line = fmt.Sprintf("%s --log-file=/var/log/gpud.log\"", line)
		}

		if endpoint != "" && !strings.Contains(line, "--endpoint=") {
			line = fmt.Sprintf("%s --endpoint=%s\"", line, endpoint)
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
