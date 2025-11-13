package ecc

import (
	"fmt"

	"github.com/leptonai/gpud/pkg/log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

type ECCErrors struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	// Aggregate counts persist across reboots (i.e. for the lifetime of the device).
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g08978d1c4fb52b6a4c72b39de144f1d9
	Aggregate AllECCErrorCounts `json:"aggregate"`

	// Volatile counts are reset each time the driver loads.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g08978d1c4fb52b6a4c72b39de144f1d9
	Volatile AllECCErrorCounts `json:"volatile"`

	// Supported is true if the ECC errors are supported by the device.
	// Set to true if any of the ECC error counts are supported.
	Supported bool `json:"supported"`
}

type AllECCErrorCounts struct {
	// Total ECC error counts for the device.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g9748430b6aa6cdbb2349c5e835d70b0f
	Total ECCErrorCounts `json:"total"`

	// GPU L1 Cache.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	L1Cache ECCErrorCounts `json:"l1_cache"`

	// GPU L2 Cache.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	L2Cache ECCErrorCounts `json:"l2_cache"`

	// Turing+ DRAM.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	DRAM ECCErrorCounts `json:"dram"`

	// Turing+ SRAM.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	SRAM ECCErrorCounts `json:"sram"`

	// GPU Device Memory.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	GPUDeviceMemory ECCErrorCounts `json:"gpu_device_memory"`

	// GPU Texture Memory.
	// Specialized memory optimized for 2D spatial locality.
	// Read-only from kernels (in most cases).
	// Optimized for specific access patterns common in graphics/image processing.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	GPUTextureMemory ECCErrorCounts `json:"gpu_texture_memory"`

	// Shared memory. Not texture memory.
	// Used for inter-thread communication and data caching within a block.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	SharedMemory ECCErrorCounts `json:"shared_memory"`

	// GPU Register File.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25
	GPURegisterFile ECCErrorCounts `json:"gpu_register_file"`
}

type ECCErrorCounts struct {
	// A memory error that was correctedFor ECC errors, these are single bit errors.
	// For Texture memory, these are errors fixed by resend.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
	Corrected uint64 `json:"corrected"`

	// A memory error that was not corrected.
	// For ECC errors, these are double bit errors.
	// For Texture memory, these are errors where the resend fails.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
	Uncorrected uint64 `json:"uncorrected"`
}

func (allCounts AllECCErrorCounts) FindUncorrectedErrs() []string {
	errs := []string{}

	if allCounts.Total.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("total uncorrected %d errors", allCounts.Total.Uncorrected))
	}
	if allCounts.L1Cache.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("L1 Cache uncorrected %d errors", allCounts.L1Cache.Uncorrected))
	}
	if allCounts.L2Cache.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("L2 Cache uncorrected %d errors", allCounts.L2Cache.Uncorrected))
	}
	if allCounts.DRAM.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("DRAM uncorrected %d errors", allCounts.DRAM.Uncorrected))
	}
	if allCounts.SRAM.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("SRAM uncorrected %d errors", allCounts.SRAM.Uncorrected))
	}
	if allCounts.GPUDeviceMemory.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("GPU device memory uncorrected %d errors", allCounts.GPUDeviceMemory.Uncorrected))
	}
	if allCounts.GPUTextureMemory.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("GPU texture memory uncorrected %d errors", allCounts.GPUTextureMemory.Uncorrected))
	}
	if allCounts.SharedMemory.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("shared memory uncorrected %d errors", allCounts.SharedMemory.Uncorrected))
	}
	if allCounts.GPURegisterFile.Uncorrected > 0 {
		errs = append(errs, fmt.Sprintf("GPU register file uncorrected %d errors", allCounts.GPURegisterFile.Uncorrected))
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func GetECCErrors(uuid string, dev device.Device, eccModeEnabledCurrent bool) (ECCErrors, error) {
	result := ECCErrors{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Aggregate: AllECCErrorCounts{},
		Volatile:  AllECCErrorCounts{},
		Supported: true,
	}

	var ret nvml.Return

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g9748430b6aa6cdbb2349c5e835d70b0f
	result.Aggregate.Total.Corrected, ret = dev.GetTotalEccErrors(
		// A memory error that was correctedFor ECC errors, these are single bit errors.
		// For Texture memory, these are errors fixed by resend.
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get total ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get total ecc errors: %s", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g9748430b6aa6cdbb2349c5e835d70b0f
	result.Aggregate.Total.Uncorrected, ret = dev.GetTotalEccErrors(
		// A memory error that was not corrected.
		// For ECC errors, these are double bit errors.
		// For Texture memory, these are errors where the resend fails.
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get total ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get total ecc errors: %s", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g9748430b6aa6cdbb2349c5e835d70b0f
	result.Volatile.Total.Corrected, ret = dev.GetTotalEccErrors(
		// A memory error that was correctedFor ECC errors, these are single bit errors.
		// For Texture memory, these are errors fixed by resend.
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get total ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get total ecc errors: %s", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g9748430b6aa6cdbb2349c5e835d70b0f
	result.Volatile.Total.Uncorrected, ret = dev.GetTotalEccErrors(
		// A memory error that was not corrected.
		// For ECC errors, these are double bit errors.
		// For Texture memory, these are errors where the resend fails.
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1gc5469bd68b9fdcf78734471d86becb24
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get total ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get total ecc errors: %s", nvml.ErrorString(ret))
	}

	if !eccModeEnabledCurrent {
		log.Logger.Debugw("ecc mode is not enabled -- skipping fetching memory error counts using 'nvmlDeviceGetMemoryErrorCounter'", "uuid", uuid)
		return result, nil
	}

	// "GetDetailedEccErrors" is deprecated, use "nvmlDeviceGetMemoryErrorCounter"
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1ga14fc137726f6c34c3351d83e3812ed4
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceEnumvs.html#group__nvmlDeviceEnumvs_1g9bcbee49054a953d333d4aa11e8b9c25

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.L1Cache.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_L1_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get l1 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get l1 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.L1Cache.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_L1_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get l1 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get l1 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.L2Cache.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_L2_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get l2 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get l2 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.L2Cache.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_L2_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get l2 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get l2 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.DRAM.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_DRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get dram cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get dram cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.DRAM.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_DRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get dram cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get dram cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.SRAM.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_SRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get sram ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get sram ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.SRAM.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_SRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get sram ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get sram ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.GPUDeviceMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_DEVICE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get gpu device memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get gpu device memory ecc errors: %s", nvml.ErrorString(ret))
	}
	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.GPUDeviceMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_DEVICE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get gpu device memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get gpu device memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.GPUTextureMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get gpu texture memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get gpu texture memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.GPUTextureMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get gpu texture memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get gpu texture memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.SharedMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_SHM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, corrected) get shared memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, corrected) failed to get shared memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Aggregate.SharedMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.AGGREGATE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_SHM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(aggregate, uncorrected) get shared memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(aggregate, uncorrected) failed to get shared memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.L1Cache.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_L1_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get l1 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get l1 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.L1Cache.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_L1_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get l1 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get l1 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.L2Cache.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_L2_CACHE,
	)
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		if ret == nvml.ERROR_NOT_SUPPORTED {
			log.Logger.Debugw("(volatile, corrected) get l2 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		} else {
			return result, fmt.Errorf("(volatile, corrected) failed to get l2 cache ecc errors: %s", nvml.ErrorString(ret))
		}
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.L2Cache.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_L2_CACHE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get l2 cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get l2 cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.DRAM.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_DRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get dram cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get dram cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.DRAM.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_DRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get dram cache ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get dram cache ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.SRAM.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_SRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get sram ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get sram ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.SRAM.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_SRAM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get sram ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get sram ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPUDeviceMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_DEVICE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get gpu device memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get gpu device memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPUDeviceMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_DEVICE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get gpu device memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get gpu device memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPUTextureMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get gpu texture memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get gpu texture memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPUTextureMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_MEMORY,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get gpu texture memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get gpu texture memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.SharedMemory.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_SHM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get shared memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get shared memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.SharedMemory.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_TEXTURE_SHM,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get shared memory ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get shared memory ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPURegisterFile.Corrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_CORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_REGISTER_FILE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, corrected) get register file ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, corrected) failed to get register file ecc errors: %s", nvml.ErrorString(ret))
	}

	// "Requires ECC Mode to be enabled."
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g30900e951fe44f1f952f0e6c89b0e2c1
	result.Volatile.GPURegisterFile.Uncorrected, ret = dev.GetMemoryErrorCounter(
		nvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		nvml.VOLATILE_ECC,
		nvml.MEMORY_LOCATION_REGISTER_FILE,
	)
	if nvmlerrors.IsNotSupportError(ret) {
		log.Logger.Debugw("(volatile, uncorrected) get register file ecc errors not supported", "error", nvml.ErrorString(ret))
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("(volatile, uncorrected) failed to get register file ecc errors: %s", nvml.ErrorString(ret))
	}

	return result, nil
}
