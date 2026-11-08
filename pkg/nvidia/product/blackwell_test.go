package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlackwellCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name string
		fm   bool
	}{
		{"NVIDIA B100", true},
		{"NVIDIA B200", true},
		{"NVIDIA GB200", false},
		{"NVIDIA GB300", false},
		{"NVIDIA B300 SXM6 AC", true},
		{"NVIDIA-GB300", false},
		{"NVIDIA-B300-SXM6-AC", true},
		{"nvidia-gB300", false},
		{"nvidia-b300-sxm6-ac", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, MemoryErrorManagementCapabilities{
				ErrorContainment: true, DynamicPageOfflining: true, RowRemapping: true,
			}, SupportedMemoryMgmtCapsByGPUProduct(tc.name))
			assert.True(t, SupportFabricStateByGPUProduct(tc.name))
			assert.Equal(t, tc.fm, SupportedFMByGPUProduct(tc.name))
		})
	}
}

func TestFabricTopologyByGPUProduct(t *testing.T) {
	for _, tc := range []struct {
		name     string
		external bool
	}{
		{"NVIDIA GB200", true},
		{"NVIDIA GB300", true},
		{"NVIDIA-GB300", true},
		{"nvidia-gb300-nvl72", true},
		{"NVIDIA B300 SXM6 AC", false},
		{"NVIDIA B200", false},
		{"NVIDIA H100 80GB HBM3", false},
		{"NVIDIA GH200", false},
		{"NVIDIA H100 PCIe", false},
		{"NVIDIA GeForce RTX 4090", false},
		{"unknown", false},
		{"", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.external, HasExternalFabricByGPUProduct(tc.name))
		})
	}
}
