package persistencemode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestDaemonPersistenceModes(t *testing.T) {
	procRoot := t.TempDir()
	firstPIDRoot := filepath.Join(procRoot, "122")
	require.NoError(t, os.MkdirAll(filepath.Join(firstPIDRoot, "fd"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(firstPIDRoot, "cmdline"), []byte("nvidia-persistenced\x00"), 0o644))
	require.NoError(t, os.Symlink("/dev/nvidia2", filepath.Join(firstPIDRoot, "fd", "3")))

	pidRoot := filepath.Join(procRoot, "123")
	require.NoError(t, os.MkdirAll(filepath.Join(pidRoot, "fd"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pidRoot, "cmdline"), []byte("/usr/bin/nvidia-persistenced\x00--persistence-mode\x00"), 0o644))
	require.NoError(t, os.Symlink("/dev/nvidia0", filepath.Join(pidRoot, "fd", "3")))
	require.NoError(t, os.Symlink("/dev/nvidia3", filepath.Join(pidRoot, "fd", "4")))
	require.NoError(t, os.Symlink("/dev/nvidiactl", filepath.Join(pidRoot, "fd", "5")))
	require.NoError(t, os.Symlink("/tmp/nvidia4", filepath.Join(pidRoot, "fd", "6")))

	modes, err := daemonPersistenceModes(procRoot)
	require.NoError(t, err)
	assert.Equal(t, map[int]bool{0: true, 2: true, 3: true}, modes)
}

func TestDaemonPersistenceModesErrors(t *testing.T) {
	_, err := daemonPersistenceModes(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)

	procRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(procRoot, "456"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(procRoot, "456", "cmdline"), []byte("unrelated\x00"), 0o644))
	_, err = daemonPersistenceModes(procRoot)
	require.Error(t, err)

	partialRoot := t.TempDir()
	partialPIDRoot := filepath.Join(partialRoot, "789")
	require.NoError(t, os.MkdirAll(filepath.Join(partialPIDRoot, "fd"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(partialPIDRoot, "cmdline"), []byte("nvidia-persistenced\x00"), 0o644))
	// Verify that a missing fd directory makes the entire snapshot indeterminate.
	require.NoError(t, os.Remove(filepath.Join(partialPIDRoot, "fd")))
	_, err = daemonPersistenceModes(partialRoot)
	require.Error(t, err)
}

func TestGetPersistenceMode(t *testing.T) {
	testCases := []struct {
		name                    string
		persistenceMode         nvml.EnableState
		persistenceModeRet      nvml.Return
		expectedPersistenceMode PersistenceMode
		expectError             bool
		expectedErrorContains   string
	}{
		{
			name:               "persistence mode enabled",
			persistenceMode:    nvml.FEATURE_ENABLED,
			persistenceModeRet: nvml.SUCCESS,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   true,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:               "persistence mode disabled",
			persistenceMode:    nvml.FEATURE_DISABLED,
			persistenceModeRet: nvml.SUCCESS,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:               "not supported",
			persistenceMode:    nvml.FEATURE_DISABLED,
			persistenceModeRet: nvml.ERROR_NOT_SUPPORTED,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: false,
			},
			expectError: false,
		},
		{
			name:                  "error case",
			persistenceMode:       nvml.FEATURE_DISABLED,
			persistenceModeRet:    nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get device persistence mode",
		},
		{
			name:                  "GPU lost error",
			persistenceMode:       nvml.FEATURE_DISABLED,
			persistenceModeRet:    nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "GPU lost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreatePersistenceModeDevice(
				"test-uuid",
				tc.persistenceMode,
				tc.persistenceModeRet,
			)

			persistenceMode, err := GetPersistenceMode("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.persistenceModeRet == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost), "Expected GPU lost error")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPersistenceMode, persistenceMode)
			}
		})
	}
}
