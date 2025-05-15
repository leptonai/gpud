package machineinfo

import (
	"fmt"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func CreateGossipRequest(machineID string, nvmlInstance nvidianvml.Instance, token string) (*apiv1.GossipRequest, error) {
	return createGossipRequest(machineID, nvmlInstance, token, GetMachineInfo)
}

func createGossipRequest(
	machineID string,
	nvmlInstance nvidianvml.Instance,
	token string,
	getMachineInfoFunc func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error),
) (*apiv1.GossipRequest, error) {
	req := &apiv1.GossipRequest{
		MachineID: machineID,
		Token:     token,
	}

	var err error
	req.MachineInfo, err = getMachineInfoFunc(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	return req, nil
}
