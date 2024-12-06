// Package host provides the host information.
package host

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

// Fetches the UUIF of the machine host, using the "dmidecode".
// ref.
// UUID=$(dmidecode -t 1 | grep -i UUID | awk '{print $2}')
func UUID(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "dmidecode", "-t", "system")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 6 && line[:6] == "UUID: " {
			uid := line[6:]
			if strings.Contains(uid, "Not Settable") {
				generateUUID, _ := uuid.NewUUID()
				uid = generateUUID.String()
			}
			return uid, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("UUID not found")
}

// VirtualizationEnvironment represents the virtualization environment of the host.
type VirtualizationEnvironment struct {
	// Type is the virtualization type.
	// Output of "systemd-detect-virt".
	// e.g., "kvm" for VM, "lxc" for container
	Type string `json:"type"`

	// Whether the host is running in a VM.
	// Output of "systemd-detect-virt --vm".
	// Set to "none" if the host is not running in a VM.
	// e.g., "kvm"
	VM string `json:"vm"`

	// Whether the host is running in a container.
	// Output of "systemd-detect-virt --container".
	// Set to "none" if the host is not running in a container.
	// e.g., "lxc"
	Container string `json:"container"`
}

// SystemdDetectVirt detects the virtualization type of the host, using "systemd-detect-virt".
func SystemdDetectVirt(ctx context.Context) (VirtualizationEnvironment, error) {
	detectExecPath, err := file.LocateExecutable("systemd-detect-virt")
	if err != nil {
		return VirtualizationEnvironment{}, nil
	}
	if detectExecPath == "" {
		return VirtualizationEnvironment{}, nil
	}

	p, err := process.New(
		process.WithBashScriptContentsToRun(fmt.Sprintf(`
%s --vm
%s --container
%s
`,
			detectExecPath,
			detectExecPath,
			detectExecPath,
		)),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return VirtualizationEnvironment{}, err
	}

	if err := p.Start(ctx); err != nil {
		return VirtualizationEnvironment{}, err
	}

	lines := make([]string, 0)

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		lines = append(lines, line)

		select {
		case err := <-p.Wait():
			if err != nil {
				return VirtualizationEnvironment{}, err
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return VirtualizationEnvironment{}, serr
		}
	}

	virt := VirtualizationEnvironment{}
	if len(lines) > 0 {
		virt.VM = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		virt.Container = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		virt.Type = strings.TrimSpace(lines[2])
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			return virt, err
		}
	case <-ctx.Done():
		return virt, ctx.Err()
	}

	return virt, nil
}

// SystemManufacturer detects the system manufacturer, using "dmidecode".
func SystemManufacturer(ctx context.Context) (string, error) {
	dmidecodePath, err := file.LocateExecutable("dmidecode")
	if err != nil {
		return "", nil
	}
	if dmidecodePath == "" {
		return "", nil
	}

	p, err := process.New(
		process.WithCommand(fmt.Sprintf("sudo %s -s system-manufacturer", dmidecodePath)),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return "", err
	}

	if err := p.Start(ctx); err != nil {
		return "", err
	}

	lines := make([]string, 0)

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		lines = append(lines, line)

		select {
		case err := <-p.Wait():
			if err != nil {
				return "", err
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return "", serr
		}
	}

	out := strings.TrimSpace(strings.Join(lines, "\n"))

	select {
	case err := <-p.Wait():
		if err != nil {
			return out, err
		}
	case <-ctx.Done():
		return out, ctx.Err()
	}

	return out, nil
}
