package fabricmanager

import (
	"context"
	"os/exec"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/netutil"
)

func TestCheckFMExists_Mockey(t *testing.T) {
	t.Run("nv-fabricmanager found", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(exec.LookPath).Return("/usr/bin/nv-fabricmanager", nil).Build()
			assert.True(t, checkFMExists())
		})
	})

	t.Run("nv-fabricmanager not found", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(exec.LookPath).Return("", assert.AnError).Build()
			assert.False(t, checkFMExists())
		})
	})
}

func TestCheckFMActive_Mockey(t *testing.T) {
	t.Run("port open", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(netutil.IsPortOpen).Return(true).Build()
			assert.True(t, checkFMActive())
		})
	})

	t.Run("port closed", func(t *testing.T) {
		mockey.PatchRun(func() {
			mockey.Mock(netutil.IsPortOpen).Return(false).Build()
			assert.False(t, checkFMActive())
		})
	})
}

func TestListPCINVSwitches_Mockey(t *testing.T) {
	// Test listPCIs directly with inline NVSwitch bridge data
	data := []byte("0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)")
	script := buildPrintScript(t, data)
	mockey.PatchRun(func() {
		// Mock LocateExecutable to return the script path (simulating lspci being found)
		mockey.Mock(file.LocateExecutable).Return(script, nil).Build()
		ctx := context.Background()
		lines, err := listPCIs(ctx, script, isNVIDIANVSwitchPCI)
		assert.NoError(t, err)
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0], "Bridge")
	})
}

func TestCountSMINVSwitches_Mockey(t *testing.T) {
	// Test countSMINVSwitches directly with inline GPU data
	data := []byte("GPU 0: NVIDIA A100-SXM4-80GB (UUID: GPU-123)\nGPU 1: NVIDIA A100-SXM4-80GB (UUID: GPU-456)")
	script := buildPrintScript(t, data)
	mockey.PatchRun(func() {
		// Mock LocateExecutable to return the script path (simulating nvidia-smi being found)
		mockey.Mock(file.LocateExecutable).Return(script, nil).Build()
		ctx := context.Background()
		lines, err := countSMINVSwitches(ctx, script)
		assert.NoError(t, err)
		assert.Len(t, lines, 2)
		assert.Contains(t, lines[0], "GPU 0")
		assert.Contains(t, lines[1], "GPU 1")
	})
}
