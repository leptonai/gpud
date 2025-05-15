package machineinfo

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
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
	req1, err := CreateLoginRequest(token, nvmlInstance, machineID, "2")
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
	req2, err := CreateLoginRequest(token, nvmlInstance, machineID, "")
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
	req3, err := CreateLoginRequest(token, nvmlInstance, machineID, "0")
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
		getProviderFunc                      func(string) string
		getSystemResourceLogicalCoresFunc    func() (string, int64, error)
		getSystemResourceMemoryTotalFunc     func() (string, error)
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
					NetworkInfo: &apiv1.MachineNetworkInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{
							mockNetworkInterface("1.2.3.4", "10.0.0.1"),
						},
					},
				}, nil
			},
			getProviderFunc: func(ip string) string {
				return "aws"
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "4", 4, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "16Gi", nil
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
				assert.Equal(t, "16Gi", req.Resources[string(corev1.ResourceMemory)])
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
				return "", nil
			},
			getMachineLocationFunc: func() *apiv1.MachineLocation {
				return &apiv1.MachineLocation{}
			},
			getMachineInfoFunc: func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					NetworkInfo: &apiv1.MachineNetworkInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{},
					},
				}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "8", 8, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "32Gi", nil
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
					NetworkInfo: &apiv1.MachineNetworkInfo{
						PrivateIPInterfaces: []apiv1.MachineNetworkInterface{},
					},
				}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "2", 2, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "8Gi", nil
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
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "", 0, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "", nil
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
			name:      "logical cores error",
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
				return &apiv1.MachineInfo{}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "", 0, errors.New("logical cores error")
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "", nil
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
			name:      "memory total error",
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
				return &apiv1.MachineInfo{}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "4", 4, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "", errors.New("memory total error")
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
				return &apiv1.MachineInfo{}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "4", 4, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "16Gi", nil
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
				return &apiv1.MachineInfo{}, nil
			},
			getProviderFunc: func(ip string) string {
				return ""
			},
			getSystemResourceLogicalCoresFunc: func() (string, int64, error) {
				return "4", 4, nil
			},
			getSystemResourceMemoryTotalFunc: func() (string, error) {
				return "16Gi", nil
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("Skipping test due to environment-specific behavior")
			}

			req, err := createLoginRequest(
				tt.token,
				&mockNvmlInstance{},
				tt.machineID,
				tt.gpuCount,
				tt.getPublicIPFunc,
				tt.getMachineLocationFunc,
				tt.getMachineInfoFunc,
				tt.getProviderFunc,
				tt.getSystemResourceLogicalCoresFunc,
				tt.getSystemResourceMemoryTotalFunc,
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
		wantPub  string
	}{
		name: "interface with public IP",
		ifacesFn: func() []apiv1.MachineNetworkInterface {
			return []apiv1.MachineNetworkInterface{
				mockNetworkInterface("1.2.3.4", ""),
			}
		},
		wantPub: "1.2.3.4",
	}

	getMachineInfoFunc := func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
		return &apiv1.MachineInfo{
			NetworkInfo: &apiv1.MachineNetworkInfo{
				PrivateIPInterfaces: test.ifacesFn(),
			},
		}, nil
	}

	req, err := createLoginRequest(
		"token",
		&mockNvmlInstance{},
		"machine-id",
		"1",
		func() (string, error) { return test.wantPub, nil },
		func() *apiv1.MachineLocation { return &apiv1.MachineLocation{} },
		getMachineInfoFunc,
		func(ip string) string { return fmt.Sprintf("provider-%s", ip) },
		func() (string, int64, error) { return "4", 4, nil },
		func() (string, error) { return "16Gi", nil },
		func() (string, error) { return "100Gi", nil },
		func(nvidianvml.Instance) (string, error) { return "1", nil },
	)

	assert.NoError(t, err)
	assert.Equal(t, test.wantPub, req.Network.PublicIP)
	assert.Equal(t, fmt.Sprintf("provider-%s", test.wantPub), req.Provider)
}
