package host

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

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

	// Whether the host is running in a KVM.
	// Set to "false" if the host is not running in a KVM.
	IsKVM bool `json:"is_kvm"`
}

// SystemdDetectVirt detects the virtualization type of the host, using "systemd-detect-virt".
func SystemdDetectVirt(ctx context.Context) (VirtualizationEnvironment, error) {
	detectExecPath, err := file.LocateExecutable("systemd-detect-virt")
	if err != nil {
		return VirtualizationEnvironment{}, nil
	}

	p, err := process.New(
		process.WithBashScriptContentsToRun(fmt.Sprintf(`
%s --vm || true
%s --container || true
%s || true
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
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return VirtualizationEnvironment{}, err
	}

	virt := VirtualizationEnvironment{}
	if len(lines) > 0 {
		virt.VM = strings.TrimSpace(lines[0])
	}
	virt.IsKVM = virt.VM == "kvm"
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

	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return "", err
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
