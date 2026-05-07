package containerd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContainerdConfigConstants tests that the containerd config constants are properly defined
func TestContainerdConfigConstants(t *testing.T) {
	// Test that the constants match the expected values
	assert.Equal(t, `default_runtime_name = "nvidia"`, containerdConfigNvidiaDefaultRuntime)
	assert.Equal(t, `plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia`, containerdConfigNvidiaRuntimePlugin)
	assert.Equal(t, `plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia`, containerdConfigNvidiaRuntimePluginV2)

	// Test that the constants can be used as byte slices
	assert.NotNil(t, []byte(containerdConfigNvidiaDefaultRuntime))
	assert.NotNil(t, []byte(containerdConfigNvidiaRuntimePlugin))
	assert.NotNil(t, []byte(containerdConfigNvidiaRuntimePluginV2))

	// Test that the constants have the expected content
	assert.Contains(t, containerdConfigNvidiaDefaultRuntime, "nvidia")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "nvidia")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "plugins")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "containerd")
	assert.Contains(t, containerdConfigNvidiaRuntimePlugin, "runtimes")
	assert.Contains(t, containerdConfigNvidiaRuntimePluginV2, "nvidia")
	assert.Contains(t, containerdConfigNvidiaRuntimePluginV2, "io.containerd.cri.v1.runtime")
}

// TestAppendImportedContainerdConfigs verifies that imports referenced in a
// containerd config are read and appended so substring checks see drop-in files.
func TestAppendImportedContainerdConfigs(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "99-nvidia.toml")
	dropInBody := `[plugins."io.containerd.cri.v1.runtime".containerd]
  default_runtime_name = "nvidia"
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"
`
	if err := os.WriteFile(dropIn, []byte(dropInBody), 0o644); err != nil {
		t.Fatalf("write drop-in: %v", err)
	}

	main := []byte("version = 3\nimports = [\"" + filepath.Join(dir, "*.toml") + "\"]\n")
	out := appendImportedContainerdConfigs(main)

	assert.Contains(t, string(out), containerdConfigNvidiaDefaultRuntime)
	assert.Contains(t, string(out), containerdConfigNvidiaRuntimePluginV2)
}

// TestAppendImportedContainerdConfigs_NoImports returns the input unchanged
// when no imports directive is present.
func TestAppendImportedContainerdConfigs_NoImports(t *testing.T) {
	in := []byte("version = 3\n[metrics]\n  address = \"0.0.0.0:1338\"\n")
	out := appendImportedContainerdConfigs(in)
	assert.Equal(t, string(in), string(out))
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
