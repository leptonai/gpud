package nvlink

import (
	"github.com/leptonai/gpud/pkg/log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type NVLink struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	// States is the list of nvlink states.
	States NVLinkStates `json:"states"`

	// Supported is true if the NVLink is supported by the device.
	Supported bool `json:"supported"`
}

type NVLinkStates []NVLinkState

func (s NVLinkStates) AllFeatureEnabled() bool {
	for _, state := range s {
		if !state.FeatureEnabled {
			return false
		}
	}
	return true
}

func (s NVLinkStates) TotalReplayErrors() uint64 {
	var total uint64
	for _, state := range s {
		total += state.ReplayErrors
	}
	return total
}

func (s NVLinkStates) TotalRecoveryErrors() uint64 {
	var total uint64
	for _, state := range s {
		total += state.RecoveryErrors
	}
	return total
}

func (s NVLinkStates) TotalCRCErrors() uint64 {
	var total uint64
	for _, state := range s {
		total += state.CRCErrors
	}
	return total
}

type NVLinkState struct {
	// Link is the nvlink link number.
	Link int `json:"link"`

	// FeatureEnabled is true if the nvlink feature is enabled.
	FeatureEnabled bool `json:"feature_enabled"`
	// ReplayErrors is the number of replay errors.
	ReplayErrors uint64 `json:"replay_errors"`
	// RecoveryErrors is the number of recovery errors.
	RecoveryErrors uint64 `json:"recovery_errors"`
	// CRCErrors is the number of crc errors.
	CRCErrors uint64 `json:"crc_errors"`

	// ThroughputRawTxBytes is the NVLink TX Data throughput + protocol overhead in bytes.
	ThroughputRawTxBytes uint64 `json:"throughput_raw_tx_bytes"`
	// ThroughputRawRxBytes is the NVLink RX Data throughput + protocol overhead in bytes.
	ThroughputRawRxBytes uint64 `json:"throughput_raw_rx_bytes"`
}

// Queries the nvlink information.
func GetNVLink(uuid string, dev device.Device) (NVLink, error) {
	nvlink := NVLink{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}

	for link := 0; link < int(nvml.NVLINK_MAX_LINKS); link++ {
		// may fail at the beginning
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1g774a9e6cb2f4897701cbc01c5a0a1f3a
		//
		// Note: DeviceGetNvLinkState reports per-link status (FEATURE_ENABLED or FEATURE_DISABLED).
		// For traditional GPUs (H100, A100): links connect directly to other GPUs
		// For GB200 B200 GPUs: links connect to external NVSwitch chips in the rack
		// When this returns FEATURE_DISABLED for all links, nvidia-smi shows:
		//   "Unable to retrieve NVLink information as all links are inActive"
		// Production case (issue #1085): B200 GPU reported all links inactive despite
		// hardware supporting NVLink. Root cause unknown but threshold detection is needed.
		state, ret := nvml.DeviceGetNvLinkState(dev, link)
		if nvmlerrors.IsNotSupportError(ret) {
			nvlink.Supported = false
			break
		}
		if nvmlerrors.IsGPULostError(ret) {
			return nvlink, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return nvlink, nvmlerrors.ErrGPURequiresReset
		}
		if ret != nvml.SUCCESS {
			log.Logger.Debugw("failed get nvlink state -- retrying", "link", link, "error", nvml.ErrorString(ret))
			continue
		}

		nvlinkState := NVLinkState{
			Link:           link,
			FeatureEnabled: state == nvml.FEATURE_ENABLED, // false when link is inactive/disabled
		}

		// e.g.,
		// nvidia-smi nvlink -e
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1gba53d5dbe3b6b25418964d77f6ff2337
		replayErrors, ret := nvml.DeviceGetNvLinkErrorCounter(dev, link, nvml.NVLINK_ERROR_DL_REPLAY)
		if ret == nvml.SUCCESS {
			nvlinkState.ReplayErrors = replayErrors
		}

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1gba53d5dbe3b6b25418964d77f6ff2337
		recoveryErrors, ret := nvml.DeviceGetNvLinkErrorCounter(dev, link, nvml.NVLINK_ERROR_DL_RECOVERY)
		if ret == nvml.SUCCESS {
			nvlinkState.RecoveryErrors = recoveryErrors
		}

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1gba53d5dbe3b6b25418964d77f6ff2337
		crcErrors, ret := nvml.DeviceGetNvLinkErrorCounter(dev, link, nvml.NVLINK_ERROR_DL_CRC_FLIT)
		if ret == nvml.SUCCESS {
			nvlinkState.CRCErrors = crcErrors
		}

		// TODO
		// nvmlDeviceGetNvLinkRemotePciInfo_v2
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1gee01cb84cd8a08f08ddaec36cd9e62ff

		nvlink.States = append(nvlink.States, nvlinkState)
	}

	return nvlink, nil
}
