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

func TestLoginRequestWorkspaceRegistrationJSON(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workspace string
	}{
		{name: "legacy registration"},
		{name: "contextual registration", workspace: "tenant-a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(LoginRequest{
				Token: "registration-token", WorkspaceID: tc.workspace, NodeGroup: "group-a",
			})
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(b, &fields); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if tc.workspace == "" {
				if string(fields["machineID"]) != `""` {
					t.Fatalf("legacy registration must retain empty machineID: %s", b)
				}
				if _, exists := fields["workspaceID"]; exists {
					t.Fatalf("legacy registration must omit workspaceID: %s", b)
				}
			} else {
				if _, exists := fields["machineID"]; exists {
					t.Fatalf("contextual registration must omit empty machineID: %s", b)
				}
				if string(fields["workspaceID"]) != `"tenant-a"` || string(fields["nodeGroup"]) != `"group-a"` {
					t.Fatalf("contextual registration lost target placement: %s", b)
				}
			}
		})
	}
}
