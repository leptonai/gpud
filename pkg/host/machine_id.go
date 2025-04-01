package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns the UUID of the machine host.
// Returns an empty string if the UUID is not found.
func GetMachineID(ctx context.Context) (string, error) {
	// hw-based UUID first
	uuid, err := DmidecodeUUID(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get UUID from dmidecode, trying to read from file", "error", err)

		// otherwise, try to read from file
		return ReadOSMachineID()
	}
	return uuid, nil
}

// Fetches the UUIF of the machine host, using the "dmidecode".
// Returns an empty string if the UUID is not found.
//
// ref.
// UUID=$(dmidecode -t 1 | grep -i UUID | awk '{print $2}')
func DmidecodeUUID(ctx context.Context) (string, error) {
	dmidecodePath, err := file.LocateExecutable("dmidecode")
	if err != nil {
		return "", errors.New("dmidecode not found")
	}

	p, err := process.New(
		process.WithCommand(fmt.Sprintf("%s -t system", dmidecodePath)),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return "", err
	}

	if err := p.Start(ctx); err != nil {
		return "", err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0)
	uuid := ""
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
			u := extractUUID(line)
			if u != "" {
				uuid = u
			}
		}),
	); err != nil {
		return "", fmt.Errorf("failed to read dmidecode for uuid: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}

	return uuid, nil
}

func extractUUID(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "UUID: ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "UUID: "))
}

// ref. https://github.com/google/cadvisor/blob/854445c010e0b634fcd855a20681ae986da235df/machine/info.go#L39
var machineIDPaths = []string{
	"/etc/machine-id",
	"/var/lib/dbus/machine-id",
}

// ReadOSMachineID returns the OS-level UUID based on /etc/machine-id or /var/lib/dbus/machine-id.
// Returns an empty string if the UUID is not found.
func ReadOSMachineID() (string, error) {
	return readOSMachineID(machineIDPaths)
}

func readOSMachineID(files []string) (string, error) {
	for _, path := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	return "", nil
}
