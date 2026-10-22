package firmware

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestGroup(t *testing.T) {
	readAt := time.Date(2026, 10, 21, 16, 0, 0, 123456789, time.FixedZone("UTC+8", 8*3600))

	var found []instance
	for _, name := range []string{"gpu10", "gpu2", "gpu1", "gpu9", "gpu3", "gpu4", "gpu5", "gpu6", "gpu7", "gpu8"} {
		found = append(found, instance{component: apiv1.FirmwareComponentGPU, version: "97.10.4A.00.0B", name: name})
	}
	found = append(found,
		instance{component: apiv1.FirmwareComponentHMCSBIOS, version: "5.36_O72OR37A_02.05.05"},
		instance{component: apiv1.FirmwareComponentDOCAHost, version: "3.2.1-044413"},
		instance{component: apiv1.FirmwareComponentCX8AWS, version: "40.47.2526", name: "mlx5_1"},
		instance{component: apiv1.FirmwareComponentCX8, version: "40.48.1102", name: "rdma_rail1"},
		instance{component: apiv1.FirmwareComponentCX8, version: "40.47.2526", name: "rdma_rail0"},
		instance{component: apiv1.FirmwareComponentBF3NIC, version: "32.47.2526", name: "mlx5_bond_0"},
	)
	rand.New(rand.NewSource(1)).Shuffle(len(found), func(i, j int) { found[i], found[j] = found[j], found[i] })

	lastUpdated := metav1.NewTime(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
	assert.Equal(t, []apiv1.MachineFirmware{
		{
			Component:     apiv1.FirmwareComponentGPU,
			Version:       "97.10.4A.00.0B",
			Instances:     10,
			InstanceNames: []string{"gpu1", "gpu2", "gpu3", "gpu4", "gpu5", "gpu6", "gpu7", "gpu8", "gpu9", "gpu10"},
			LastUpdated:   lastUpdated,
		},
		{Component: apiv1.FirmwareComponentBF3NIC, Version: "32.47.2526", Instances: 1, InstanceNames: []string{"mlx5_bond_0"}, LastUpdated: lastUpdated},
		{Component: apiv1.FirmwareComponentCX8, Version: "40.47.2526", Instances: 1, InstanceNames: []string{"rdma_rail0"}, LastUpdated: lastUpdated},
		{Component: apiv1.FirmwareComponentCX8, Version: "40.48.1102", Instances: 1, InstanceNames: []string{"rdma_rail1"}, LastUpdated: lastUpdated},
		{Component: apiv1.FirmwareComponentCX8AWS, Version: "40.47.2526", Instances: 1, InstanceNames: []string{"mlx5_1"}, LastUpdated: lastUpdated},
		{Component: apiv1.FirmwareComponentDOCAHost, Version: "3.2.1-044413", Instances: 1, LastUpdated: lastUpdated},
		{Component: apiv1.FirmwareComponentHMCSBIOS, Version: "5.36_O72OR37A_02.05.05", Instances: 1, LastUpdated: lastUpdated},
	}, group(found, readAt))
}

func TestGroup_Empty(t *testing.T) {
	assert.Nil(t, group(nil, time.Now()))
	assert.Nil(t, group([]instance{}, time.Now()))
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		less bool
	}{
		{"gpu2", "gpu10", true},
		{"gpu10", "gpu2", false},
		{"mlx5_9", "mlx5_10", true},
		{"mlx5_10", "mlx5_9", false},
		{"rdma_rail0", "rdma_rail1", true},
		{"mlx5_0", "mlx5_bond_0", true},
		{"a", "a1", true},
		{"a1", "a", false},
		{"gpu1", "gpu01", true},
		{"gpu01", "gpu1", false},
		{"gpu1", "gpu1", false},
		{"", "a", true},
		{"a", "", false},
		{"10", "9a", false},
		{"x99999999999999999999999", "x100000000000000000000000", true},
	}
	for _, tt := range tests {
		t.Run(tt.a+"<"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.less, naturalLess(tt.a, tt.b))
		})
	}

	names := []string{"mlx5_10", "mlx5_2", "mlx5_bond_0", "mlx5_1", "mlx5_0", "mlx5_11"}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	assert.Equal(t, []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_10", "mlx5_11", "mlx5_bond_0"}, names)
}
