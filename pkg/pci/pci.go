package pci

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"

	"sigs.k8s.io/yaml"
)

// Lists all PCI devices.
func List(ctx context.Context, opts ...OpOption) (Devices, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	lspciPath, err := file.LocateExecutable("lspci")
	if err != nil {
		return nil, nil
	}

	p, err := process.New(
		process.WithBashScriptContentsToRun(fmt.Sprintf("sudo %s -vvv", lspciPath)),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(p.StdoutReader())
	devs, err := parseLspciVVV(ctx, scanner, op.nameMatchFunc)
	if err != nil {
		return nil, err
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return devs, nil
}

type Devices []Device

func (devs Devices) JSON() ([]byte, error) {
	return json.Marshal(devs)
}

func (devs Devices) YAML() ([]byte, error) {
	return yaml.Marshal(devs)
}

type Device struct {
	// ID of the PCI device.
	// e.g., "00:0e.0" in "00:0e.0 PCI ..."
	ID string `json:"id"`

	// Name that comes after the ID.
	// e.g., "3D controller" in "00:0e.0 3D controller"
	Name string `json:"name"`

	// Access control service.
	// e.g., Capabilities: [170 v1] Access Control Services
	AccessControlService *AccessControlService `json:"access_control_service,omitempty"`

	// Kernel driver in use.
	// e.g., Kernel driver in use: pcieport
	KernelDriverInUse string `json:"kernel_driver_in_use,omitempty"`

	// Kernel modules.
	// e.g., Kernel modules: nvidiafb, nouveau, nvidia_drm, nvidia
	KernelModules []string `json:"kernel_modules,omitempty"`
}

type AccessControlService struct {
	ACSCap ACS `json:"acs_cap"`

	// Access Control Service Control, useful to validate bare metal PCI config.
	// e.g.,
	// If this device sets SrcValid+ (SrcValid: true), then ACS is enabled:
	// "IO virtualization (also known as VT-d or IOMMU) can interfere with GPU Direct
	// by redirecting all PCI point-to-point traffic to the CPU root complex,
	// causing a significant performance reduction or even a hang.
	// If PCI switches have ACS enabled (on baremetal systems), it needs to be disabled."
	// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
	ACSCtl ACS `json:"acs_ctl"`
}

// e.g.,
// ACSCap:	SrcValid+ TransBlk+ ReqRedir- CmpltRedir- UpstreamFwd+ EgressCtrl- DirectTrans-
// ACSCtl:	SrcValid- TransBlk- ReqRedir- CmpltRedir- UpstreamFwd- EgressCtrl- DirectTrans-
type ACS struct {
	SrcValid    bool `json:"src_valid"`
	TransBlk    bool `json:"trans_blk"`
	ReqRedir    bool `json:"req_redir"`
	CmpltRedir  bool `json:"cmplt_redir"`
	UpstreamFwd bool `json:"upstream_fwd"`
	EgressCtrl  bool `json:"egress_ctrl"`
	DirectTrans bool `json:"direct_trans"`
}

func ParseACS(s string) ACS {
	a := ACS{}
	fields := strings.Fields(s)
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		enabled := field[len(field)-1] == '+'
		switch field[:len(field)-1] {
		case "SrcValid":
			a.SrcValid = enabled
		case "TransBlk":
			a.TransBlk = enabled
		case "ReqRedir":
			a.ReqRedir = enabled
		case "CmpltRedir":
			a.CmpltRedir = enabled
		case "UpstreamFwd":
			a.UpstreamFwd = enabled
		case "EgressCtrl":
			a.EgressCtrl = enabled
		case "DirectTrans":
			a.DirectTrans = enabled
		}
	}
	return a
}

const (
	// regex for PCI device header/first line
	// no leading whitespace, the ID always in these formats:
	// [HEXADECIMAL]:[HEXADECIMAL].[HEXADECIMAL] [any string ...]
	//
	// e.g.,
	// 00:0e.0 PCI ...
	// 00:14.0 USB ...
	// 85:00.0 Bridge ...
	// ec:00.0 System ...
	// ff:1e.7 System
	pciDeviceHeaderRegex = `^[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]{1} [^\s]+`

	// regex for "Access Control Services", which may have leading spaces or tabs
	// e.g.,
	// Capabilities: [220 v1] Access Control Services
	// Capabilities: [230 v1] Access Control Services
	// Capabilities: [170 v1] Access Control Services
	capAccessControlServicesRegex = `^[ \t]*Capabilities: \[\w+ v\d+\] Access Control Services`

	// regex for "Kernel driver in use", which may have leading spaces or tabs
	// e.g.,
	// Kernel driver in use: pcieport
	// Kernel driver in use: nvidia
	// Kernel driver in use: nvidia test
	// Kernel driver in use: mlx5_core
	kernelDriverInUseRegex = `^[ \t]*Kernel driver in use: [\w\s]+`

	// regex for "Kernel modules", which may have leading spaces or tabs
	// e.g.,
	// Kernel modules: isst_if_mbox_pci
	// Kernel modules: nvidiafb, nouveau, nvidia_drm, nvidia
	// Kernel modules: mlx5_core
	kernelModulesRegex = `^[ \t]*Kernel modules: [\w,\s]+`
)

var (
	pciDeviceHeaderRegexCompiled          = regexp.MustCompile(pciDeviceHeaderRegex)
	capAccessControlServicesRegexCompiled = regexp.MustCompile(capAccessControlServicesRegex)
	kernelDriverInUseRegexCompiled        = regexp.MustCompile(kernelDriverInUseRegex)
	kernelModulesRegexCompiled            = regexp.MustCompile(kernelModulesRegex)
)

func parseLspciVVV(ctx context.Context, scanner *bufio.Scanner, nameMatchFunc func(string) bool) (Devices, error) {
	devs := []Device{}

	curDev := &Device{}
	allIDs := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Text()
		_ = line

		// found new device start
		if pciDeviceHeaderRegexCompiled.MatchString(line) {
			splits := strings.Fields(line)
			if len(splits) < 2 {
				return nil, fmt.Errorf("invalid PCI device header: %q (expected at least 2 splits)", line)
			}
			id := strings.TrimSpace(splits[0])
			name := strings.TrimSpace(strings.Join(splits[1:], " "))

			// append before we reset
			// "Kernel driver in use:" but no "Kernel modules:"
			if curDev.ID != "" {
				devs = append(devs, *curDev)
				allIDs[curDev.ID] = struct{}{}
			}

			// reset the current device
			curDev = &Device{
				ID:   id,
				Name: name,
			}
			continue
		}

		// parsing starts from "Access Control Services"
		// e.g., "Capabilities: [230 v1] Access Control Services"
		if capAccessControlServicesRegexCompiled.MatchString(line) {
			curDev.AccessControlService = &AccessControlService{}
			continue
		}

		// curDev.AccessControlService != nil: "Capabilities: [230 v1] Access Control Services" was found in the previous line
		if curDev.AccessControlService != nil && strings.HasPrefix(strings.TrimSpace(strings.TrimSpace(line)), "ACSCap:") {
			curDev.AccessControlService.ACSCap = ParseACS(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "ACSCap:")))
			continue
		}
		// curDev.AccessControlService != nil : "Capabilities: [230 v1] Access Control Services" was found in the previous line
		// e.g., ACSCtl:	SrcValid- TransBlk- ReqRedir- CmpltRedir- UpstreamFwd- EgressCtrl- DirectTrans-
		if curDev.AccessControlService != nil && strings.HasPrefix(strings.TrimSpace(strings.TrimSpace(line)), "ACSCtl:") {
			curDev.AccessControlService.ACSCtl = ParseACS(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "ACSCtl:")))
			continue
		}

		// "Kernel driver in use:"
		if kernelDriverInUseRegexCompiled.MatchString(line) {
			trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Kernel driver in use:"))
			curDev.KernelDriverInUse = trimmed
			continue
		}

		// parsing ends at "Kernel driver in use:" or "Kernel modules:"
		// e.g., "Kernel driver in use: pcieport"
		if kernelModulesRegexCompiled.MatchString(line) {
			trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Kernel modules:"))
			curDev.KernelModules = strings.Fields(trimmed)

			// we are at the end of the device
			devs = append(devs, *curDev)
			curDev = &Device{}
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return nil, serr
		}
	}

	_, ok := allIDs[curDev.ID]
	if curDev.ID != "" && !ok {
		devs = append(devs, *curDev)
		allIDs[curDev.ID] = struct{}{}
	}

	filtered := []Device{}
	for _, dev := range devs {
		if nameMatchFunc != nil && !nameMatchFunc(dev.Name) {
			continue
		}
		filtered = append(filtered, dev)
	}

	return filtered, nil
}
