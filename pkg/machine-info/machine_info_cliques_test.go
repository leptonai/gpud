package machineinfo

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestMachineFabricTypedCliqueAgreement(t *testing.T) {
	first := completedFabricState("cluster", 65534)
	first.Cliques = []apiv1.GPUFabricClique{{Type: 0, ID: 65534}, {Type: 1, ID: 65534}, {Type: 2, ID: 32766}, {Type: 3, ID: 32766}}
	second := first
	cluster, legacy, cliques := getGPUFabricIdentifiers(newFabricMachineInfoInstance(first, second))
	require.Equal(t, "cluster", cluster)
	require.Equal(t, uint32(65534), *legacy)
	require.Equal(t, first.Cliques, cliques)
	info := apiv1.MachineInfo{GPUInfo: &apiv1.MachineGPUInfo{ClusterUUID: cluster, CliqueID: legacy, Cliques: cliques}}
	var out bytes.Buffer
	info.RenderTable(&out)
	for _, label := range []string{"Unicast Pointer", "Multicast Pointer", "Unicast LE", "Multicast LE", "65534", "32766"} {
		require.Contains(t, out.String(), label)
	}
	payload, err := json.Marshal(info)
	require.NoError(t, err)
	var decoded apiv1.MachineInfo
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, info.GPUInfo, decoded.GPUInfo)
	// Same legacy pointer clique, but different logical endpoint domains.
	second.Cliques = append([]apiv1.GPUFabricClique(nil), first.Cliques...)
	second.Cliques[2].ID = 123
	for i := 0; i < 20; i++ {
		cluster, legacy, cliques = getGPUFabricIdentifiers(newFabricMachineInfoInstance(first, second))
		require.Empty(t, cluster)
		require.Nil(t, legacy)
		require.Nil(t, cliques)
	}
	// V4 may report logical endpoints without any legacy pointer assignment.
	first.Cliques = first.Cliques[2:]
	cluster, legacy, cliques = getGPUFabricIdentifiers(newFabricMachineInfoInstance(first))
	require.Equal(t, "cluster", cluster)
	require.Nil(t, legacy)
	require.Equal(t, first.Cliques, cliques)
}
