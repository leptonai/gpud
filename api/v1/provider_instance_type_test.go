package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderInstanceTypeWireContract(t *testing.T) {
	for _, value := range []string{"", "p5.48xlarge", "Standard_ND96isr_H100_v5", "a3-highgpu-8g"} {
		for _, request := range []any{
			LoginRequest{ProviderInstanceType: value}, GossipRequest{ProviderInstanceType: value},
		} {
			data, err := json.Marshal(request)
			require.NoError(t, err)
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &raw))
			if value == "" {
				assert.NotContains(t, raw, "providerInstanceType")
				continue
			}
			assert.JSONEq(t, `"`+value+`"`, string(raw["providerInstanceType"]))
			var login LoginRequest
			require.NoError(t, json.Unmarshal(data, &login))
			assert.Equal(t, value, login.ProviderInstanceType)
			var gossip GossipRequest
			require.NoError(t, json.Unmarshal(data, &gossip))
			assert.Equal(t, value, gossip.ProviderInstanceType)
		}
	}
}
