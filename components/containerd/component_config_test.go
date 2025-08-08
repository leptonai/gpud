package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContainerdConfigConstants tests that the containerd config constants are properly defined
func TestContainerdConfigConstants(t *testing.T) {
	// Test that the constants match the expected values
	assert.Equal(t, `default_runtime_name = "nvidia"`, containerdConfigNvidiaDefaultRuntime)
	assert.Equal(t, `plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia`, containerdConfigNvidiaRuntimePlugin)

	// Test that the constants can be used as byte slices
	assert.NotNil(t, []byte(containerdConfigNvidiaDefaultRuntime))
	assert.NotNil(t, []byte(containerdConfigNvidiaRuntimePlugin))

	// Test that the constants have the expected content
	assert.Contains(t, containerdConfigNvidiaDefaultRuntime, "nvidia")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "nvidia")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "plugins")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "containerd")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "runtimes")
}

// TestContainerdConfigWithBothNvidiaSettings tests configs that should contain both nvidia settings
func TestContainerdConfigWithBothNvidiaSettings(t *testing.T) {
	// Example of a valid config with both settings
	validConfig := `
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "nvidia"

[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  privileged_without_host_devices = false
  runtime_engine = ""
  runtime_root = ""
  runtime_type = "io.containerd.runc.v2"
`

	// Test that a valid config contains both required strings
	configBytes := []byte(validConfig)
	assert.Contains(t, string(configBytes), containerdConfigNvidiaDefaultRuntime)
	assert.Contains(t, string(configBytes), containerdConfigNvidiaRuntimePlugin)
}
