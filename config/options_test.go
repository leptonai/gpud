package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOp_ApplyOpts(t *testing.T) {
	t.Run("empty options", func(t *testing.T) {
		op := &Op{}
		err := op.ApplyOpts(nil)
		assert.NoError(t, err)
	})

	t.Run("multiple options", func(t *testing.T) {
		op := &Op{}
		err := op.ApplyOpts([]OpOption{
			WithFilesToCheck("/path1", "/path2"),
			WithKernelModulesToCheck("mod1", "mod2"),
			WithDockerIgnoreConnectionErrors(true),
			WithKubeletIgnoreConnectionErrors(true),
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"/path1", "/path2"}, op.FilesToCheck)
		assert.Equal(t, []string{"mod1", "mod2"}, op.KernelModulesToCheck)
		assert.True(t, op.DockerIgnoreConnectionErrors)
		assert.True(t, op.KubeletIgnoreConnectionErrors)
	})
}

func TestWithFilesToCheck(t *testing.T) {
	tests := []struct {
		name          string
		initialFiles  []string
		filesToAdd    []string
		expectedFiles []string
	}{
		{
			name:          "add to empty",
			initialFiles:  nil,
			filesToAdd:    []string{"/path1", "/path2"},
			expectedFiles: []string{"/path1", "/path2"},
		},
		{
			name:          "add to existing",
			initialFiles:  []string{"/existing"},
			filesToAdd:    []string{"/new1", "/new2"},
			expectedFiles: []string{"/existing", "/new1", "/new2"},
		},
		{
			name:          "empty addition",
			initialFiles:  []string{"/existing"},
			filesToAdd:    []string{},
			expectedFiles: []string{"/existing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{FilesToCheck: tt.initialFiles}
			opt := WithFilesToCheck(tt.filesToAdd...)
			opt(op)
			assert.Equal(t, tt.expectedFiles, op.FilesToCheck)
		})
	}
}

func TestWithKernelModulesToCheck(t *testing.T) {
	tests := []struct {
		name            string
		initialModules  []string
		modulesToAdd    []string
		expectedModules []string
	}{
		{
			name:            "add to empty",
			initialModules:  nil,
			modulesToAdd:    []string{"mod1", "mod2"},
			expectedModules: []string{"mod1", "mod2"},
		},
		{
			name:            "add to existing",
			initialModules:  []string{"existing"},
			modulesToAdd:    []string{"new1", "new2"},
			expectedModules: []string{"existing", "new1", "new2"},
		},
		{
			name:            "empty addition",
			initialModules:  []string{"existing"},
			modulesToAdd:    []string{},
			expectedModules: []string{"existing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{KernelModulesToCheck: tt.initialModules}
			opt := WithKernelModulesToCheck(tt.modulesToAdd...)
			opt(op)
			assert.Equal(t, tt.expectedModules, op.KernelModulesToCheck)
		})
	}
}

func TestWithDockerIgnoreConnectionErrors(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{"set true", true, true},
		{"set false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithDockerIgnoreConnectionErrors(tt.value)
			opt(op)
			assert.Equal(t, tt.expected, op.DockerIgnoreConnectionErrors)
		})
	}
}

func TestWithKubeletIgnoreConnectionErrors(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{"set true", true, true},
		{"set false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithKubeletIgnoreConnectionErrors(tt.value)
			opt(op)
			assert.Equal(t, tt.expected, op.KubeletIgnoreConnectionErrors)
		})
	}
}

func TestWithNvidiaSMICommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"set custom path", "/usr/local/bin/nvidia-smi", "/usr/local/bin/nvidia-smi"},
		{"set empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithNvidiaSMICommand(tt.path)
			opt(op)
			assert.Equal(t, tt.expected, op.NvidiaSMICommand)
		})
	}
}

func TestWithNvidiaSMIQueryCommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"set custom path", "/usr/local/bin/nvidia-smi-query", "/usr/local/bin/nvidia-smi-query"},
		{"set empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithNvidiaSMIQueryCommand(tt.path)
			opt(op)
			assert.Equal(t, tt.expected, op.NvidiaSMIQueryCommand)
		})
	}
}

func TestWithIbstatCommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"set custom path", "/usr/local/bin/ibstat", "/usr/local/bin/ibstat"},
		{"set empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithIbstatCommand(tt.path)
			opt(op)
			assert.Equal(t, tt.expected, op.IbstatCommand)
		})
	}
}
