package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluationWireCompatibility(t *testing.T) {
	for _, tc := range []struct {
		value Evaluation
		want  string
	}{{Evaluation{}, `{}`}, {Evaluated(), `{"evaluated":true}`}, {NotEvaluated(NotEvaluatedNoData), `{"evaluated":false,"not_evaluated_reason":"no_data_yet"}`}} {
		b, err := json.Marshal(tc.value)
		require.NoError(t, err)
		require.JSONEq(t, tc.want, string(b))
	}
}
func TestAffectedGPUsDeterministic(t *testing.T) {
	s := HealthState{Reason: "fault on GPU-b", ExtraInfo: map[string]string{"data": "retained"}}
	s.SetAffectedGPUs("GPU-b", "GPU-a", "GPU-a", "")
	require.JSONEq(t, `["GPU-a","GPU-b"]`, s.ExtraInfo["gpu_uuids"])
	require.Equal(t, "retained", s.ExtraInfo["data"])
	require.Contains(t, s.Reason, "GPU-a")
}
func TestPlatformZeroCoordinatesRemainPresent(t *testing.T) {
	zero := uint8(0)
	b, err := json.Marshal(GPUPlatformInfo{SlotNumber: &zero, ModuleID: &zero})
	require.NoError(t, err)
	require.Contains(t, string(b), `"slotNumber":0`)
	require.Contains(t, string(b), `"moduleId":0`)
	require.NotContains(t, string(b), `"trayIndex"`)
}
