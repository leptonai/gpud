package nvml

import (
	"github.com/leptonai/gpud/pkg/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type NVLink struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

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

func (s NVLinkStates) TotalRelayErrors() uint64 {
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
		Supported: true,
	}

	for link := 0; link < int(nvml.NVLINK_MAX_LINKS); link++ {
		// may fail at the beginning
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__NvLink.html#group__NvLink_1g774a9e6cb2f4897701cbc01c5a0a1f3a
		state, ret := nvml.DeviceGetNvLinkState(dev, link)
		if IsNotSupportError(ret) {
			nvlink.Supported = false
			break
		}
		if IsGPULostError(ret) {
			return nvlink, ErrGPULost
		}
		if ret != nvml.SUCCESS {
			log.Logger.Debugw("failed get nvlink state -- retrying", "link", link, "error", nvml.ErrorString(ret))
			continue
		}

		nvlinkState := NVLinkState{
			Link:           link,
			FeatureEnabled: state == nvml.FEATURE_ENABLED,
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
