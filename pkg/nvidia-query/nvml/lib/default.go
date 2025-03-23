package lib

import (
	"os"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvml_lib_mock "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib/mock"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	EnvMockAllSuccess              = "GPUD_NVML_MOCK_ALL_SUCCESS"
	EnvInjectRemapedRowsPending    = "GPUD_NVML_INJECT_REMAPPED_ROWS_PENDING"
	EnvInjectClockEventsHwSlowdown = "GPUD_NVML_INJECT_CLOCK_EVENTS_HW_SLOWDOWN"
)

// 0x0000000000000000 is none
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
const (
	reasonHWSlowdown           uint64 = 0x0000000000000008
	reasonSwThermalSlowdown    uint64 = 0x0000000000000020
	reasonHWSlowdownThermal    uint64 = 0x0000000000000040
	reasonHWSlowdownPowerBrake uint64 = 0x0000000000000080
)

var clockEventsToInjectHwSlowdown = reasonHWSlowdown | reasonSwThermalSlowdown | reasonHWSlowdownThermal | reasonHWSlowdownPowerBrake

func NewDefault(options ...OpOption) Library {
	opts := []OpOption{}

	if os.Getenv(EnvMockAllSuccess) == "true" {
		opts = append(opts,
			WithNVML(nvml_lib_mock.AllSuccessInterface),
			WithPropertyExtractor(nvml_lib_mock.HasNvmlPropertyExtractor),
		)
	}

	if os.Getenv(EnvInjectRemapedRowsPending) == "true" {
		opts = append(opts,
			WithDeviceGetRemappedRowsForAllDevs(func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return) {
				log.Logger.Infow("injecting remapped rows pending", "corrRows", 0, "uncRows", 10, "isPending", true, "failureOccurred", false)
				return 0, 10, true, false, nvml.SUCCESS
			}),
		)
	}

	if os.Getenv(EnvInjectClockEventsHwSlowdown) == "true" {
		opts = append(opts,
			WithDeviceGetCurrentClocksEventReasonsForAllDevs(func() (uint64, nvml.Return) {
				log.Logger.Infow("injecting clock events hw slowdown", "reasons", clockEventsToInjectHwSlowdown)
				return clockEventsToInjectHwSlowdown, nvml.SUCCESS
			}),
		)
	}

	return New(append(opts, options...)...)
}
