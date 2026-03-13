package v1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoginRequestNodeLabelsJSON(t *testing.T) {
	t.Run("omits nodeLabels when nil", func(t *testing.T) {
		req := LoginRequest{
			Token:              "token",
			Provider:           "aws",
			ProviderInstanceID: "i-123",
		}

		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if strings.Contains(string(b), `"nodeLabels"`) {
			t.Fatalf("Marshal() unexpectedly included nodeLabels: %s", string(b))
		}
	})

	t.Run("round trips populated nodeLabels", func(t *testing.T) {
		req := LoginRequest{
			Token:              "token",
			MachineID:          "machine-123",
			NodeGroup:          "group-a",
			NodeLabels:         map[string]string{"team": "ml", "rack": "r42"},
			Provider:           "aws",
			ProviderInstanceID: "i-123",
		}

		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		var got LoginRequest
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		if got.NodeLabels["team"] != "ml" || got.NodeLabels["rack"] != "r42" {
			t.Fatalf("unexpected nodeLabels after round trip: %#v", got.NodeLabels)
		}
	})

	t.Run("keeps explicit empty nodeLabels", func(t *testing.T) {
		req := LoginRequest{
			Token:              "token",
			NodeLabels:         map[string]string{},
			Provider:           "aws",
			ProviderInstanceID: "i-123",
		}

		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if !strings.Contains(string(b), `"nodeLabels":{}`) {
			t.Fatalf("Marshal() did not preserve explicit empty nodeLabels: %s", string(b))
		}
	})
}
