package host

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns the UUID of the machine host.
// Returns an empty string if the UUID is not found.
func GetMachineID(ctx context.Context) (string, error) {
	// hw-based UUID first
	uuid, err := GetDmidecodeUUID(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get UUID from dmidecode, trying to read from file", "error", err)

		// otherwise, try to read from file
		return GetOSMachineID()
	}
	return uuid, nil
}

// Fetches the UUIF of the machine host, using the "dmidecode".
// Returns an empty string if the UUID is not found.
//
// ref.
// UUID=$(dmidecode -t 1 | grep -i UUID | awk '{print $2}')
func GetDmidecodeUUID(ctx context.Context) (string, error) {
	dmidecodePath, err := file.LocateExecutable("dmidecode")
	if err != nil {
		return "", errors.New("dmidecode not found")
	}

	p, err := process.New(
		process.WithCommand(fmt.Sprintf("%s -t system", dmidecodePath)),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
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

// GetOSMachineID returns the OS-level UUID based on /etc/machine-id or /var/lib/dbus/machine-id.
// Returns an empty string if the UUID is not found.
func GetOSMachineID() (string, error) {
	return getOSMachineID(machineIDPaths)
}

func getOSMachineID(files []string) (string, error) {
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

// GetOSName reads the os name from the /etc/os-release file.
func GetOSName() (string, error) {
	return getOSName("/etc/os-release")
}

func getOSName(file string) (string, error) {
	if _, err := os.Stat(file); err != nil {
		return "", err
	}

	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	name := ""
	prettyName := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "NAME=") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "NAME="))
			name = strings.TrimSpace(strings.Trim(name, "\""))
		}
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = strings.TrimSpace(strings.TrimPrefix(line, "PRETTY_NAME="))
			prettyName = strings.TrimSpace(strings.Trim(prettyName, "\""))
		}
	}
	if prettyName != "" {
		return prettyName, nil
	}
	return name, nil
}

const (
	dmiDir       = "/sys/class/dmi"
	ppcDevTree   = "/proc/device-tree"
	s390xDevTree = "/etc" // s390/s390x changes
)

// GetSystemUUID returns the system UUID of the machine.
// ref. https://github.com/google/cadvisor/blob/master/utils/sysfs/sysfs.go#L442
func GetSystemUUID() (string, error) {
	if id, err := os.ReadFile(path.Join(dmiDir, "id", "product_uuid")); err == nil {
		return strings.TrimSpace(string(id)), nil
	} else if id, err = os.ReadFile(path.Join(ppcDevTree, "system-id")); err == nil {
		return strings.TrimSpace(strings.TrimRight(string(id), "\000")), nil
	} else if id, err = os.ReadFile(path.Join(ppcDevTree, "vm,uuid")); err == nil {
		return strings.TrimSpace(strings.TrimRight(string(id), "\000")), nil
	} else if id, err = os.ReadFile(path.Join(s390xDevTree, "machine-id")); err == nil {
		return strings.TrimSpace(string(id)), nil
	} else {
		return "", err
	}
}
