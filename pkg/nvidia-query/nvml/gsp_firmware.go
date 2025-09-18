package nvml

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// GSPFirmwareMode is the GSP firmware mode of the device.
// ref. https://www.nvidia.com.tw/Download/driverResults.aspx/224886/tw
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g37f644e70bd4853a78ca2bbf70861f67
type GSPFirmwareMode struct {
	UUID      string `json:"uuid"`
	BusID     string `json:"bus_id"`
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
}

func GetGSPFirmwareMode(uuid string, dev device.Device) (GSPFirmwareMode, error) {
	mode := GSPFirmwareMode{
		UUID:  uuid,
		BusID: dev.PCIBusID(),
	}

	gspEnabled, supported, ret := dev.GetGspFirmwareMode()
	if IsNotSupportError(ret) {
		mode.Enabled = false
		mode.Supported = false
		return mode, nil
	}
	if IsGPULostError(ret) {
		return mode, ErrGPULost
	}
	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return mode, fmt.Errorf("failed to get gsp firmware mode: %v", nvml.ErrorString(ret))
	}
	mode.Enabled = gspEnabled
	mode.Supported = supported

	return mode, nil
}

// ValidateGSPFirmwareModeWithKernelConfig validates the GSP firmware mode against kernel module configuration.
// This is necessary because NVML may report GSP as enabled even when it's disabled at the kernel level.
//
// Example of the discrepancy:
// $ cat /etc/modprobe.d/nvidia.conf
// options nvidia NVreg_EnableGpuFirmware=0
//
// $ nvidia-smi -q | grep "GSP"
// GSP Firmware Version : N/A
//
// In this case, even though the kernel module has GSP disabled (NVreg_EnableGpuFirmware=0),
// NVML might still report GSP as enabled. This function checks the kernel configuration
// and overrides the NVML result when the kernel explicitly disables GSP.
//
// ref. https://docs.nvidia.com/vgpu/latest/grid-vgpu-user-guide/index.html#disabling-gsp
func ValidateGSPFirmwareModeWithKernelConfig(mode GSPFirmwareMode, kernelConfigPath string) GSPFirmwareMode {
	// If NVML already says GSP is disabled, no need to check further
	if !mode.Enabled {
		return mode
	}

	f, err := os.Open(kernelConfigPath)
	if err != nil {
		// If we can't read the config file, trust NVML but log it
		if !os.IsNotExist(err) {
			log.Logger.Warnw("failed to open kernel module config file -- reverting back to NVML result",
				"path", kernelConfigPath,
				"error", err,
				"nvml_gsp_enabled", mode.Enabled,
			)
		}
		return mode
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "options nvidia") || strings.HasPrefix(line, "options	nvidia") {
			if strings.Contains(line, "NVreg_EnableGpuFirmware=0") {
				// kernel has GSP explicitly disabled, override NVML result
				log.Logger.Warnw("NVML reports GSP firmware as enabled but kernel module config has it disabled, overriding to disabled",
					"uuid", mode.UUID,
					"bus_id", mode.BusID,
					"kernel_config_path", kernelConfigPath,
					"kernel_config_line", line,
					"nvml_gsp_enabled", true,
					"corrected_gsp_enabled", false,
				)
				mode.Enabled = false
				return mode
			}
			// If NVreg_EnableGpuFirmware=1 or not present, trust NVML
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Logger.Warnw("error reading kernel module config file, trusting NVML result",
			"path", kernelConfigPath,
			"error", err,
			"nvml_gsp_enabled", mode.Enabled,
		)
	}

	return mode
}
