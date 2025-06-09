package machineinfo

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/providers"
)

// TestCreateLoginRequest tests the login request creation
func TestCreateLoginRequest(t *testing.T) {
	if os.Getenv("TEST_CREATE_LOGIN_REQUEST") != "true" {
		t.Skip("TEST_CREATE_LOGIN_REQUEST is not set")
	}

	// Skip if NVML is not available
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		t.Skip("NVML not available, skipping test")
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	// Test parameters
	token := "test-token"
	machineID := "test-machine-id"

	// Test with GPU count specified
	req1, err := CreateLoginRequest(token, machineID, "", "2", nvmlInstance)
	if err != nil {
		t.Skipf("Could not create login request with GPU count: %v", err)
	}

	// Validate request fields
	assert.Equal(t, token, req1.Token)
	assert.Equal(t, machineID, req1.MachineID)
	assert.NotNil(t, req1.Location)
	assert.NotNil(t, req1.MachineInfo)

	// Check resources
	assert.NotEmpty(t, req1.Resources[string(corev1.ResourceCPU)])
	assert.NotEmpty(t, req1.Resources[string(corev1.ResourceMemory)])
	assert.NotEmpty(t, req1.Resources[string(corev1.ResourceEphemeralStorage)])
	assert.Equal(t, "2", req1.Resources["nvidia.com/gpu"])

	// Test without GPU count specified (auto-detect)
	req2, err := CreateLoginRequest(token, machineID, "", "", nvmlInstance)
	if err != nil {
		t.Skipf("Could not create login request without GPU count: %v", err)
	}

	// Check that GPU resources were set based on auto-detection
	if gpuCount, err := GetSystemResourceGPUCount(nvmlInstance); err == nil && gpuCount != "0" {
		assert.Equal(t, gpuCount, req2.Resources["nvidia.com/gpu"])
	} else {
		// If no GPUs or error, the nvidia.com/gpu resource should not be present
		_, hasGPU := req2.Resources["nvidia.com/gpu"]
		assert.False(t, hasGPU)
	}

	// Test with no IPs specified
	req3, err := CreateLoginRequest(token, machineID, "", "0", nvmlInstance)
	if err != nil {
		t.Skipf("Could not create login request without IPs: %v", err)
	}

	// Since no GPUs (count "0"), nvidia.com/gpu should not be present
	_, hasGPU := req3.Resources["nvidia.com/gpu"]
	assert.False(t, hasGPU)
}

type mockNvmlInstance struct {
	nvidianvml.Instance
}

func (m *mockNvmlInstance) Devices() map[string]device.Device {
	return make(map[string]device.Device)
}

func (m *mockNvmlInstance) ProductName() string {
	return ""
}

func (m *mockNvmlInstance) Brand() string {
	return ""
}

func (m *mockNvmlInstance) Architecture() string {
	return ""
}

// mockNetworkInterface creates a network interface with specified IP values
func mockNetworkInterface(publicIP, privateIP string) apiv1.MachineNetworkInterface {
	var addr netip.Addr
	if privateIP != "" {
		addr = netip.MustParseAddr(privateIP)
	} else if publicIP != "" {
		addr = netip.MustParseAddr(publicIP)
	}

	ip := publicIP
	if privateIP != "" {
		ip = privateIP
	}

	return apiv1.MachineNetworkInterface{
		Interface: "eth0",
		MAC:       "00:11:22:33:44:55",
		IP:        ip,
		Addr:      addr,
	}
}

func TestCreateLoginRequest_Basic(t *testing.T) {
	tests := []struct {
		name                                 string
		token                                string
		machineID                            string
		gpuCount                             string
		getPublicIPFunc                      func() (string, error)
		getMachineLocationFunc               func() *apiv1.MachineLocation
		getMachineInfoFunc                   func(nvidianvml.Instance) (*apiv1.MachineInfo, error)
		getProviderFunc                      func(string) *providers.Info
		getSystemResourceRootVolumeTotalFunc func() (string, error)
		getSystemResourceGPUCountFunc        func(nvidianvml.Instance) (string, error)
		wantErr                              bool
		validate                             func(*testing.T, *apiv1.LoginRequest)
		skip                                 bool
	}{
		{
			name:      "success case with no private IP validation",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "",
			getPublicIPFunc: func() (string, error) {
				return "1.2.3.4", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{
					Region: "us-east-1",
					Zone:   "us-east-1a",
				}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 4,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 16 * 1024 * 1024 * 1024, // 16GB
					},
					NICInfo: &apiv1.MachineNICInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{
							mockNetworkInterface("1.2.3.4", "10.0.0.1"),
						},
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "100Gi", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "2", nil
			},
			wantErr: false,
			validate: func(t *testing.T, req *apiv1.LoginRequest) {
				assert.Equal(t, "test-token", req.Token)
				assert.Equal(t, "test-machine-id", req.MachineID)
				assert.Equal(t, "us-east-1", req.Location.Region)
				assert.Equal(t, "us-east-1a", req.Location.Zone)
				assert.Equal(t, "1.2.3.4", req.Network.PublicIP)
				// We don't validate PrivateIP as it depends on the result of Is4()
				assert.Equal(t, "aws", req.Provider)
				assert.Equal(t, "4", req.Resources[string(corev1.ResourceCPU)])
				// Memory is calculated from bytes, so we need to check the actual value
				memQty, err := resource.ParseQuantity(req.Resources[string(corev1.ResourceMemory)])
				assert.NoError(t, err)
				expectedMem := resource.NewQuantity(16*1024*1024*1024, resource.DecimalSI)
				assert.Equal(t, expectedMem.String(), memQty.String())
				assert.Equal(t, "100Gi", req.Resources[string(corev1.ResourceEphemeralStorage)])
				assert.Equal(t, "2", req.Resources["nvidia.com/gpu"])
			},
			skip: false,
		},
		{
			name:      "explicit gpu count",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "3", // Explicit GPU count
			getPublicIPFunc: func() (string, error) {
				return "5.6.7.8", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 8,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 32 * 1024 * 1024 * 1024, // 32GB
					},
					NICInfo: &apiv1.MachineNICInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{},
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "200Gi", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "0", nil // This should be ignored since gpuCount is set explicitly
			},
			wantErr: false,
			validate: func(t *testing.T, req *apiv1.LoginRequest) {
				assert.Equal(t, "3", req.Resources["nvidia.com/gpu"])
				assert.Equal(t, "8", req.Resources[string(corev1.ResourceCPU)])
				// Memory is calculated from bytes, so we need to check the actual value
				memQty, err := resource.ParseQuantity(req.Resources[string(corev1.ResourceMemory)])
				assert.NoError(t, err)
				expectedMem := resource.NewQuantity(32*1024*1024*1024, resource.DecimalSI)
				assert.Equal(t, expectedMem.String(), memQty.String())
			},
			skip: false,
		},
		{
			name:      "zero gpu count",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "0",
			getPublicIPFunc: func() (string, error) {
				return "", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 2,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 8 * 1024 * 1024 * 1024, // 8GB
					},
					NICInfo: &apiv1.MachineNICInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{},
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "50Gi", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "0", nil
			},
			wantErr: false,
			validate: func(t *testing.T, req *apiv1.LoginRequest) {
				_, exists := req.Resources["nvidia.com/gpu"]
				assert.False(t, exists, "GPU count should not be in resources when it's zero")
				assert.Equal(t, "2", req.Resources[string(corev1.ResourceCPU)])
				// Memory is calculated from bytes, so we need to check the actual value
				memQty, err := resource.ParseQuantity(req.Resources[string(corev1.ResourceMemory)])
				assert.NoError(t, err)
				expectedMem := resource.NewQuantity(8*1024*1024*1024, resource.DecimalSI)
				assert.Equal(t, expectedMem.String(), memQty.String())
			},
			skip: false,
		},
		{
			name:      "machine info error",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "",
			getPublicIPFunc: func() (string, error) {
				return "", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return nil, errors.New("machine info error")
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "", nil
			},
			wantErr: true,
			skip:    false,
		},
		{
			name:      "root volume total error",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "",
			getPublicIPFunc: func() (string, error) {
				return "", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 4,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 16 * 1024 * 1024 * 1024,
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "", errors.New("root volume total error")
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "", nil
			},
			wantErr: true,
			skip:    false,
		},
		{
			name:      "gpu count error",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "",
			getPublicIPFunc: func() (string, error) {
				return "", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 4,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 16 * 1024 * 1024 * 1024,
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "aws", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "100Gi", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "", errors.New("gpu count error")
			},
			wantErr: true,
			skip:    false,
		},
		{
			name:      "public ip error",
			token:     "test-token",
			machineID: "test-machine-id",
			gpuCount:  "1",
			getPublicIPFunc: func() (string, error) {
				return "", errors.New("public ip error")
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 4,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 16 * 1024 * 1024 * 1024,
					},
				}, nil
			},
			getProviderFunc: func(ip string) *providers.Info {
				return &providers.Info{Provider: "unknown", PublicIP: ip}
			},
			getSystemResourceRootVolumeTotalFunc: func() (string, error) {
				return "100Gi", nil
			},
			getSystemResourceGPUCountFunc: func(nvidianvml.Instance) (string, error) {
				return "1", nil
			},
			wantErr: false, // Public IP error is logged but doesn't fail the request
			validate: func(t *testing.T, req *apiv1.LoginRequest) {
				assert.Equal(t, "", req.Network.PublicIP)
				assert.Equal(t, "unknown", req.Provider)
				assert.Equal(t, "1", req.Resources["nvidia.com/gpu"])
			},
			skip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("Skipping test due to environment-specific behavior")
			}

			req, err := createLoginRequest(
				tt.token,
				tt.machineID,
				"",
				tt.gpuCount,
				&mockNvmlInstance{},
				tt.getPublicIPFunc,
				tt.getMachineLocationFunc,
				tt.getMachineInfoFunc,
				tt.getProviderFunc,
				tt.getSystemResourceRootVolumeTotalFunc,
				tt.getSystemResourceGPUCountFunc,
			)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, req)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, req)
				if tt.validate != nil {
					tt.validate(t, req)
				}
			}
		})
	}
}

func TestCreateLoginRequest_NetworkBasics(t *testing.T) {
	// This test focuses only on the public IP part which doesn't depend on Is4()
	test := struct {
		name     string
		ifacesFn func() []apiv1.MachineNetworkInterface
	}{
		name: "interface with public IP",
		ifacesFn: func() []apiv1.MachineNetworkInterface {
			return []apiv1.MachineNetworkInterface{
				mockNetworkInterface("1.2.3.4", ""),
			}
		},
	}

	getMachineInfoFunc := func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
		return &apiv1.MachineInfo{
			CPUInfo: &apiv1.MachineCPUInfo{
				LogicalCores: 4,
			},
			MemoryInfo: &apiv1.MachineMemoryInfo{
				TotalBytes: 16 * 1024 * 1024 * 1024,
			},
			NICInfo: &apiv1.MachineNICInfo{
				PrivateIPInterfaces: test.ifacesFn(),
			},
		}, nil
	}

	req, err := createLoginRequest(
		"token",
		"machine-id",
		"",
		"1",
		&mockNvmlInstance{},
		func() (string, error) { return "1.2.3.4", nil },
		func() *apiv1.MachineLocation { return &apiv1.MachineLocation{} },
		getMachineInfoFunc,
		func(ip string) *providers.Info {
			return &providers.Info{Provider: fmt.Sprintf("provider-%s", ip), PublicIP: ip}
		},
		func() (string, error) { return "100Gi", nil },
		func(nvidianvml.Instance) (string, error) { return "1", nil },
	)

	assert.NoError(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, "1.2.3.4", req.Network.PublicIP)
	assert.Equal(t, "provider-1.2.3.4", req.Provider)
	assert.Equal(t, "1", req.Resources["nvidia.com/gpu"])
}

// TestCreateLoginRequest_PrivateIPDetection tests private IP detection logic
func TestCreateLoginRequest_PrivateIPDetection(t *testing.T) {
	tests := []struct {
		name        string
		interfaces  []apiv1.MachineNetworkInterface
		expectedIP  string
		description string
	}{
		{
			name: "private IPv4 detected",
			interfaces: []apiv1.MachineNetworkInterface{
				{
					Interface: "eth0",
					MAC:       "00:11:22:33:44:55",
					IP:        "10.0.0.1",
					Addr:      netip.MustParseAddr("10.0.0.1"),
				},
			},
			expectedIP:  "10.0.0.1",
			description: "Should detect private IPv4 address",
		},
		{
			name: "no private IP when only public",
			interfaces: []apiv1.MachineNetworkInterface{
				{
					Interface: "eth0",
					MAC:       "00:11:22:33:44:55",
					IP:        "8.8.8.8",
					Addr:      netip.MustParseAddr("8.8.8.8"),
				},
			},
			expectedIP:  "",
			description: "Should not detect public IP as private",
		},
		{
			name: "first private IPv4 selected",
			interfaces: []apiv1.MachineNetworkInterface{
				{
					Interface: "eth0",
					MAC:       "00:11:22:33:44:55",
					IP:        "192.168.1.1",
					Addr:      netip.MustParseAddr("192.168.1.1"),
				},
				{
					Interface: "eth1",
					MAC:       "00:11:22:33:44:56",
					IP:        "10.0.0.1",
					Addr:      netip.MustParseAddr("10.0.0.1"),
				},
			},
			expectedIP:  "192.168.1.1",
			description: "Should select first private IPv4 address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getMachineInfoFunc := func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: 4,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: 16 * 1024 * 1024 * 1024,
					},
					NICInfo: &apiv1.MachineNICInfo{
						PrivateIPInterfaces: tt.interfaces,
					},
				}, nil
			}

			req, err := createLoginRequest(
				"token",
				"machine-id",
				"",
				"1",
				&mockNvmlInstance{},
				func() (string, error) { return "1.2.3.4", nil },
				func() *apiv1.MachineLocation { return &apiv1.MachineLocation{} },
				getMachineInfoFunc,
				func(ip string) *providers.Info { return &providers.Info{Provider: "provider"} },
				func() (string, error) { return "100Gi", nil },
				func(nvidianvml.Instance) (string, error) { return "1", nil },
			)

			assert.NoError(t, err, tt.description)
			assert.NotNil(t, req, tt.description)
			assert.Equal(t, tt.expectedIP, req.Network.PrivateIP, tt.description)
		})
	}
}

// TestCreateLoginRequest_ResourceCalculation tests resource calculation
func TestCreateLoginRequest_ResourceCalculation(t *testing.T) {
	tests := []struct {
		name           string
		cpuCores       int64
		memoryBytes    uint64
		expectedCPU    string
		expectedMemory string
	}{
		{
			name:           "small machine",
			cpuCores:       2,
			memoryBytes:    4 * 1024 * 1024 * 1024, // 4GB
			expectedCPU:    "2",
			expectedMemory: "4294967296", // 4GB in bytes
		},
		{
			name:           "large machine",
			cpuCores:       64,
			memoryBytes:    256 * 1024 * 1024 * 1024, // 256GB
			expectedCPU:    "64",
			expectedMemory: "274877906944", // 256GB in bytes
		},
		{
			name:           "zero resources",
			cpuCores:       0,
			memoryBytes:    0,
			expectedCPU:    "0",
			expectedMemory: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getMachineInfoFunc := func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					CPUInfo: &apiv1.MachineCPUInfo{
						LogicalCores: tt.cpuCores,
					},
					MemoryInfo: &apiv1.MachineMemoryInfo{
						TotalBytes: tt.memoryBytes,
					},
				}, nil
			}

			req, err := createLoginRequest(
				"token",
				"machine-id",
				"",
				"0",
				&mockNvmlInstance{},
				func() (string, error) { return "", nil },
				func() *apiv1.MachineLocation { return &apiv1.MachineLocation{} },
				getMachineInfoFunc,
				func(ip string) *providers.Info { return &providers.Info{Provider: ""} },
				func() (string, error) { return "100Gi", nil },
				func(nvidianvml.Instance) (string, error) { return "0", nil },
			)

			assert.NoError(t, err)
			assert.NotNil(t, req)
			assert.Equal(t, tt.expectedCPU, req.Resources[string(corev1.ResourceCPU)])
			assert.Equal(t, tt.expectedMemory, req.Resources[string(corev1.ResourceMemory)])
		})
	}
}
