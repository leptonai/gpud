package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// Mock NVML instance for testing
type mockNvmlInstance struct{}

func (m *mockNvmlInstance) NVMLExists() bool                  { return true }
func (m *mockNvmlInstance) Library() nvmllib.Library          { return nil }
func (m *mockNvmlInstance) Devices() map[string]device.Device { return nil }
func (m *mockNvmlInstance) ProductName() string               { return "test-gpu" }
func (m *mockNvmlInstance) Architecture() string              { return "test-arch" }
func (m *mockNvmlInstance) Brand() string                     { return "test-brand" }
func (m *mockNvmlInstance) DriverVersion() string             { return "test-version" }
func (m *mockNvmlInstance) DriverMajor() int                  { return 1 }
func (m *mockNvmlInstance) CUDAVersion() string               { return "test-cuda" }
func (m *mockNvmlInstance) FabricManagerSupported() bool      { return false }
func (m *mockNvmlInstance) FabricStateSupported() bool        { return false }
func (m *mockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNvmlInstance) Shutdown() error { return nil }
func (m *mockNvmlInstance) InitError() error { return nil }

// Tests for processGossip
func TestProcessGossip(t *testing.T) {
	t.Run("nil createGossipRequestFunc", func(t *testing.T) {
		session := &Session{
			createGossipRequestFunc: nil,
		}
		resp := &Response{}

		session.processGossip(resp)

		// Should return early without setting anything
		assert.Nil(t, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("successful gossip request creation", func(t *testing.T) {
		expectedGossipReq := &apiv1.GossipRequest{
			MachineID: "test-machine-id",
		}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Equal(t, "test-machine-id", machineID)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "test-machine-id",
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("error in gossip request creation", func(t *testing.T) {
		expectedError := errors.New("failed to create gossip request")

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			return nil, expectedError
		}

		session := &Session{
			machineID:               "test-machine-id",
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Nil(t, resp.GossipRequest)
		assert.Equal(t, expectedError.Error(), resp.Error)
	})

	t.Run("with nvml instance", func(t *testing.T) {
		mockNvmlInstance := &mockNvmlInstance{}
		expectedGossipReq := &apiv1.GossipRequest{
			MachineID: "test-machine-id",
		}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Equal(t, "test-machine-id", machineID)
			assert.Equal(t, mockNvmlInstance, nvmlInstance)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "test-machine-id",
			nvmlInstance:            mockNvmlInstance,
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("empty machine ID and token", func(t *testing.T) {
		expectedGossipReq := &apiv1.GossipRequest{}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Empty(t, machineID)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "",
			token:                   "",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})
}
