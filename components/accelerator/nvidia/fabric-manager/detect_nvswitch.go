// Package fabricmanager provides utilities for detecting and managing NVIDIA NVSwitch devices.
//
// # NVSwitch and Fabric Manager
//
// NVIDIA NVSwitch is a physical interconnect switch that enables high-bandwidth GPU-to-GPU
// communication in multi-GPU systems. Fabric Manager is the user-space software daemon that
// manages and configures NVSwitch hardware.
//
// Fabric Manager is specifically designed to manage NVSwitch hardware, and NVSwitch hardware
// requires Fabric Manager to function properly. Without Fabric Manager running:
//   - NVSwitch devices remain uninitialized
//   - GPU-to-GPU communication through NVSwitch is unavailable
//   - Multi-GPU workloads cannot utilize the full NVLink fabric bandwidth
//
// The relationship between them:
//   - NVSwitch: Physical hardware switch chips (PCIe bridge devices)
//   - Fabric Manager: Software service that initializes, configures, and monitors NVSwitch
//
// # Official Documentation
//
// For more information about Fabric Manager and NVSwitch, see:
//   - NVIDIA Fabric Manager User Guide: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html
//   - NVIDIA NVSwitch Documentation: https://www.nvidia.com/en-us/data-center/nvlink/
//
// # Supported Systems
//
// Fabric Manager is required for NVSwitch-based systems including:
//   - NVIDIA DGX systems (DGX A100, DGX H100, DGX GB200)
//   - NVIDIA HGX systems (HGX A100, HGX H100, HGX H200)
//   - Systems with multiple GPUs connected via NVSwitch
//
// # Detection Methods
//
// This package provides two methods for detecting NVSwitch hardware:
//  1. PCIe enumeration via lspci (ListPCINVSwitches function)
//  2. nvidia-smi nvlink status query (CountSMINVSwitches function)
//
// Both methods are used as fallbacks to ensure robust NVSwitch detection across
// different system configurations.
package fabricmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// DeviceVendorID defines the NVIDIA PCI vendor ID.
// This is used to filter PCI devices to only NVIDIA hardware.
//
// Example usage with lspci:
//
//	lspci -nn | grep -i "10de.*"
//
// Reference: https://devicehunt.com/view/type/pci/vendor/10DE
const DeviceVendorID = "10de"

// ListPCINVSwitches returns all lspci lines that represent NVIDIA NVSwitch devices.
//
// NVSwitch devices appear as PCI bridge devices in the lspci output. This function
// enumerates all NVIDIA bridge devices which typically represent NVSwitch hardware.
//
// NVSwitch is the physical interconnect hardware that connects multiple GPUs in a
// high-performance fabric. Without NVSwitch, GPUs cannot communicate efficiently in
// multi-GPU configurations.
//
// Example lspci output for NVSwitch:
//
//	Older format (A100-era):
//	  0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)
//
//	Newer format (GB200 and later):
//	  0018:00:00.0 PCI bridge [0604]: NVIDIA Corporation Device [10de:22b1]
//
// This function is the primary method for detecting NVSwitch hardware via PCIe
// enumeration. It's used by the Fabric Manager component to determine if NVSwitch
// is present and therefore if Fabric Manager service is required.
func ListPCINVSwitches(ctx context.Context) ([]string, error) {
	return listPCIs(ctx, "lspci -nn", isNVIDIANVSwitchPCI)
}

// isNVIDIANVSwitchPCI determines if a lspci output line represents an NVSwitch device.
//
// NVSwitch devices are identified as NVIDIA bridge devices in lspci output.
// The function performs case-insensitive matching to handle different lspci output formats.
//
// Matches:
//   - "Bridge" devices (older A100/H100 systems)
//   - "PCI bridge" devices (newer GB200 systems)
//
// Example matching lines:
//
//	0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)
//	0018:00:00.0 PCI bridge [0604]: NVIDIA Corporation Device [10de:22b1]
//
// Does NOT match:
//   - GPU devices (3D controller, VGA controller)
//   - Non-NVIDIA bridge devices (Intel, AMD, etc.)
func isNVIDIANVSwitchPCI(line string) bool {
	line = strings.ToLower(line)
	return strings.Contains(line, "nvidia") && strings.Contains(line, "bridge")
}

// listPCIs executes an lspci command and filters the output using the provided match function.
//
// This is a generic helper function for enumerating PCI devices. It:
//  1. Locates the lspci executable
//  2. Executes the lspci command
//  3. Filters output to only NVIDIA devices (vendor ID 10de)
//  4. Applies the custom match function to identify specific device types
//
// The matchFunc parameter allows callers to specify custom logic for identifying
// specific types of NVIDIA devices (e.g., GPUs vs NVSwitch bridges).
//
// This function is used internally by ListPCINVSwitches to enumerate NVSwitch devices.
func listPCIs(ctx context.Context, command string, matchFunc func(line string) bool) ([]string, error) {
	lspciPath, err := file.LocateExecutable(strings.Split(command, " ")[0])
	if lspciPath == "" || err != nil {
		return nil, fmt.Errorf("failed to locate lspci: %w", err)
	}

	p, err := process.New(
		process.WithCommand(command),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			// Only process lines containing NVIDIA vendor ID
			if !strings.Contains(strings.ToLower(line), DeviceVendorID) {
				return
			}

			// Apply the custom match function to identify specific device types
			if matchFunc != nil && matchFunc(line) {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read lspci output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}

// CountSMINVSwitches queries nvidia-smi to count GPUs with NVLink connections,
// which indicates the presence of NVSwitch hardware.
//
// This function uses "nvidia-smi nvlink --status" to enumerate GPUs that have
// NVLink connections. In systems with NVSwitch, all GPUs will be connected
// through the NVSwitch fabric.
//
// Returns a list of GPU description lines, where each line represents a GPU
// with NVLink connectivity. The number of lines indicates the number of GPUs
// connected to the NVSwitch fabric.
//
// This is used as a fallback detection method when PCIe enumeration (lspci)
// is unavailable or unreliable.
//
// Example output line:
//
//	GPU 7: NVIDIA A100-SXM4-80GB (UUID: GPU-754035b4-4708-efcd-b261-623aea38bcad)
func CountSMINVSwitches(ctx context.Context) ([]string, error) {
	return countSMINVSwitches(ctx, "nvidia-smi nvlink --status")
}

// countSMINVSwitches executes the given nvidia-smi command and parses the output
// to count GPUs with NVLink connections.
//
// The function looks for lines containing:
//   - "GPU " - indicates a GPU entry
//   - "NVIDIA" - confirms it's an NVIDIA GPU
//   - "UUID" - ensures it's a valid GPU description line
//
// This pattern matches the standard nvidia-smi output format for NVLink status.
func countSMINVSwitches(ctx context.Context, command string) ([]string, error) {
	execPath, err := file.LocateExecutable(strings.Split(command, " ")[0])
	if execPath == "" || err != nil {
		return nil, fmt.Errorf("failed to locate nvidia-smi: %w", err)
	}

	p, err := process.New(
		process.WithCommand(command),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			// Match GPU description lines from nvidia-smi nvlink --status output
			// Example: GPU 7: NVIDIA A100-SXM4-80GB (UUID: GPU-754035b4-4708-efcd-b261-623aea38bcad)
			if strings.Contains(line, "GPU ") && strings.Contains(line, "NVIDIA") && strings.Contains(line, "UUID") {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read nvidia-smi nvlink output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}
