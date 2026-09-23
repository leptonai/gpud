package machineinfo

import (
	"fmt"

	apiv1 "github.com/leptonai/gpud/api/v1"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
)

func CreateGossipRequest(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
	return createGossipRequest(machineID, nvmlInstance, GetMachineInfo)
}

func CreateGossipRequestWithContainerd(machineID string, nvmlInstance nvidianvml.Instance, cfg configcommon.ContainerdConfig) (*apiv1.GossipRequest, error) {
	if cfg.IsZero() {
		return CreateGossipRequest(machineID, nvmlInstance)
	}
	return createGossipRequest(machineID, nvmlInstance, func(instance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
		return GetMachineInfoWithContainerd(instance, cfg)
	})
}

func createGossipRequest(
	machineID string,
	nvmlInstance nvidianvml.Instance,
	getMachineInfoFunc func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error),
) (*apiv1.GossipRequest, error) {
	req := &apiv1.GossipRequest{
		MachineID: machineID,
	}

	var err error
	req.MachineInfo, err = getMachineInfoFunc(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	return req, nil
}
