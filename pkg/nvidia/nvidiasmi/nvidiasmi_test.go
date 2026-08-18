package nvidiasmi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChassisSerial(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "gb200 nvl72 output",
			output: `GPU 0000000F:00:00.0
    Product Name                    : NVIDIA GB200
    Product Brand                   : NVIDIA
    Serial Number                   : 1651524001234
    GPU UUID                        : GPU-11111111-2222-3333-4444-555555555555
    Chassis Serial Number           : 3136434J5234567
`,
			expected: "3136434J5234567",
		},
		{
			name: "value not available",
			output: `GPU 00000001:00:00.0
    Product Name                    : NVIDIA H100 80GB HBM3
    Chassis Serial Number           : N/A
`,
			expected: "",
		},
		{
			name:     "field absent on non-NVL platforms",
			output:   "GPU 00000001:00:00.0\n    Product Name                    : NVIDIA H100 80GB HBM3\n",
			expected: "",
		},
		{
			name:     "empty output",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseChassisSerial(tt.output))
		})
	}
}

// writeFakeNvidiaSmi writes a fake "nvidia-smi" script to a temporary
// directory and returns the directory, for use with t.Setenv("PATH", dir).
func writeFakeNvidiaSmi(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nvidia-smi"), []byte(script), 0o755))
	return dir
}

func TestGetChassisSerial_WithStubBinary(t *testing.T) {
	t.Run("returns the chassis serial number", func(t *testing.T) {
		dir := writeFakeNvidiaSmi(t, "#!/bin/sh\nprintf 'GPU 0000000F:00:00.0\n    Chassis Serial Number           : 3136434J5234567\n'\n")
		t.Setenv("PATH", dir)

		serial, err := GetChassisSerial(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "3136434J5234567", serial)
	})

	t.Run("returns empty when the platform reports no chassis serial", func(t *testing.T) {
		dir := writeFakeNvidiaSmi(t, "#!/bin/sh\nprintf 'GPU 00000001:00:00.0\n    Product Name                    : NVIDIA H100 80GB HBM3\n'\n")
		t.Setenv("PATH", dir)

		serial, err := GetChassisSerial(context.Background())
		require.NoError(t, err)
		assert.Empty(t, serial)
	})

	t.Run("returns error when nvidia-smi is not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		_, err := GetChassisSerial(context.Background())
		require.Error(t, err)
	})

	t.Run("returns error when nvidia-smi fails", func(t *testing.T) {
		dir := writeFakeNvidiaSmi(t, "#!/bin/sh\necho 'some error' >&2\nexit 1\n")
		t.Setenv("PATH", dir)

		_, err := GetChassisSerial(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nvidia-smi -q failed")
	})
}
