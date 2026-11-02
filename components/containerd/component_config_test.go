package containerd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasNvidiaRuntimeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{
			name: "v2 nvidia as default runtime",
			config: `version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: true,
		},
		{
			name: "v3 nvidia as default runtime",
			config: `version = 3
[plugins."io.containerd.cri.v1.runtime".containerd]
  default_runtime_name = "nvidia"
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: true,
		},
		{
			name: "v2 native CDI",
			config: `version = 2
[plugins."io.containerd.grpc.v1.cri"]
  enable_cdi = true
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: true,
		},
		{
			name: "native CDI table after its subtable with runc as default",
			config: `[plugins."io.containerd.cri.v1.runtime".containerd]
  default_runtime_name = "runc"
[plugins."io.containerd.cri.v1.runtime"]
  enable_cdi=true # enabled by GPU Operator
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: true,
		},
		{
			// k3s/RKE2 version 2 template with --default-runtime=nvidia.
			name: "k3s v2 template quotes the runtime key",
			config: `version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  snapshotter = "overlayfs"
  default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes."nvidia"]
  runtime_type = "io.containerd.runc.v2"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes."nvidia".options]
  BinaryName = "/usr/local/nvidia/toolkit/nvidia-container-runtime"
  SystemdCgroup = true`,
			want: true,
		},
		{
			// containerd 2.x enables CDI by default; the k3s/RKE2 version 3
			// template quotes keys with single quotes and leaves enable_cdi unset.
			name: "k3s v3 template relies on default CDI",
			config: `version = 3
[plugins.'io.containerd.cri.v1.runtime']
  enable_selinux = false
[plugins.'io.containerd.cri.v1.runtime'.containerd]
  snapshotter = "overlayfs"
[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.'nvidia']
  runtime_type = "io.containerd.runc.v2"
[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.'nvidia'.options]
  BinaryName = "/usr/local/nvidia/toolkit/nvidia-container-runtime"
[[plugins.'io.containerd.transfer.v1.local'.unpack_config]]
  platform = "linux/amd64"
  snapshotter = "overlayfs"`,
			want: true,
		},
		{
			name: "v3 with CDI explicitly disabled",
			config: `version = 3
[plugins.'io.containerd.cri.v1.runtime']
  enable_cdi = false
[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.'nvidia']
  runtime_type = "io.containerd.runc.v2"`,
			want: false,
		},
		{
			name: "v2 nvidia handler without default or native CDI",
			config: `version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "runc"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes."nvidia"]
  runtime_type = "io.containerd.runc.v2"`,
			want: false,
		},
		{
			name: "native CDI without nvidia runtime handler",
			config: `[plugins."io.containerd.cri.v1.runtime"]
  enable_cdi = true`,
			want: false,
		},
		{
			name: "commented settings are ignored",
			config: `# default_runtime_name = "nvidia"
# enable_cdi = true
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: false,
		},
		{
			name: "default runtime outside the CRI plugin is ignored",
			config: `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`,
			want: false,
		},
		{
			name:   "no nvidia configuration",
			config: `[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc]`,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasNvidiaRuntimeConfiguration([]byte(tt.config))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasNvidiaRuntimeConfigurationInvalidConfig(t *testing.T) {
	_, err := hasNvidiaRuntimeConfiguration([]byte(`[plugins."io.containerd.grpc.v1.cri"`))
	require.Error(t, err)
}

// Drop-ins written by nvidia-container-toolkit are merged after the main
// config, so their explicit settings override it.
func TestHasNvidiaRuntimeConfigurationImports(t *testing.T) {
	writeDropIn := func(t *testing.T, dir, name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	nvidiaDropIn := `version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"
`

	t.Run("drop-in supplies the nvidia runtime", func(t *testing.T) {
		dir := t.TempDir()
		writeDropIn(t, dir, "99-nvidia.toml", nvidiaDropIn)
		writeDropIn(t, dir, "00-broken.toml", "[unterminated")
		main := "version = 2\nimports = [\"" + filepath.Join(dir, "*.toml") + "\"]\n" +
			"[plugins.\"io.containerd.grpc.v1.cri\".containerd]\n  default_runtime_name = \"runc\"\n"
		got, err := hasNvidiaRuntimeConfiguration([]byte(main))
		require.NoError(t, err)
		assert.True(t, got, "an invalid sibling drop-in must not hide the valid one")
	})

	t.Run("drop-in disables default CDI", func(t *testing.T) {
		dir := t.TempDir()
		writeDropIn(t, dir, "99-cdi.toml", "version = 3\n[plugins.'io.containerd.cri.v1.runtime']\n  enable_cdi = false\n")
		main := "version = 3\nimports = [\"" + filepath.Join(dir, "*.toml") + "\"]\n" +
			"[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.'nvidia']\n  runtime_type = \"io.containerd.runc.v2\"\n"
		got, err := hasNvidiaRuntimeConfiguration([]byte(main))
		require.NoError(t, err)
		assert.False(t, got)
	})
}
