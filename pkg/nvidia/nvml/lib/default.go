package lib

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	EnvMockAllSuccess              = "GPUD_NVML_MOCK_ALL_SUCCESS"
	EnvInjectRemapedRowsPending    = "GPUD_NVML_INJECT_REMAPPED_ROWS_PENDING"
	EnvInjectClockEventsHwSlowdown = "GPUD_NVML_INJECT_CLOCK_EVENTS_HW_SLOWDOWN"
)

var ErrNVMLNotFound = errors.New("NVML not found")

// New instantiates a new NVML instance and initializes the NVML library.
// It returns nil and error, if NVML is not supported.
// It also injects the mock data if the environment variables are set.
func New(opts ...OpOption) (Library, error) {
	if os.Getenv(EnvMockAllSuccess) == "true" {
		opts = append(opts,
			WithNVML(allSuccessInterface),
			WithPropertyExtractor(hasNvmlPropertyExtractor),
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
				log.Logger.Infow("injecting clock events hw slowdown")
				return reasonHWSlowdown | reasonSwThermalSlowdown | reasonHWSlowdownThermal | reasonHWSlowdownPowerBrake, nvml.SUCCESS
			}),
		)
	}

	lib := createLibrary(opts...)
	ret := lib.NVML().Init()
	if ret == nvml.SUCCESS {
		return lib, nil
	}
	if ret == nvml.ERROR_LIBRARY_NOT_FOUND {
		return nil, ErrNVMLNotFound
	}

	// e.g., "Driver Not Loaded"
	es := nvml.ErrorString(ret)
	if strings.Contains(strings.ToLower(es), "driver not loaded") {
		return nil, ErrNVMLNotFound
	}

	return nil, fmt.Errorf("failed to initialize NVML: %s", es)
}

// 0x0000000000000000 is none
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
const (
	reasonHWSlowdown           uint64 = 0x0000000000000008
	reasonSwThermalSlowdown    uint64 = 0x0000000000000020
	reasonHWSlowdownThermal    uint64 = 0x0000000000000040
	reasonHWSlowdownPowerBrake uint64 = 0x0000000000000080
)
