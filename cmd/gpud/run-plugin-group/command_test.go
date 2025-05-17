package runplugingroup

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func TestCmdRunPluginGroup(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/components/trigger-tag" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("tagName") == "" {
			http.Error(w, "missing tag name", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"components":["test1","test2"],"exit":0,"success":true}`)); err != nil {
			t.Logf("Error writing response: %v", err)
		}
	}))
	defer server.Close()

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "missing tag argument",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "too many arguments",
			args:        []string{"tag1", "tag2"},
			expectError: true,
		},
		{
			name:        "valid tag argument",
			args:        []string{"--server", server.URL, "valid-tag"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Commands = []cli.Command{
				{
					Name:   "run-plugin-group",
					Action: Command,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "server",
							Value: "http://localhost:8080",
							Usage: "Server URL",
						},
					},
				},
			}

			err := app.Run(append([]string{"gpud", "run-plugin-group"}, tt.args...))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
