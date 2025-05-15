package command

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
					Action: cmdRunPluginGroup,
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

func TestCmdListPlugins(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/components/custom-plugin" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Logf("Error writing response: %v", err)
		}
	}))
	defer server.Close()

	tests := []struct {
		name        string
		args        []string
		serverURL   string
		expectError bool
	}{
		{
			name:        "no arguments",
			args:        []string{"--server", server.URL},
			serverURL:   server.URL,
			expectError: false,
		},
		{
			name:        "with extra arguments",
			args:        []string{"--server", server.URL, "extra"},
			serverURL:   server.URL,
			expectError: false, // Extra arguments are ignored
		},
		{
			name:        "custom server URL",
			args:        []string{"--server", server.URL},
			serverURL:   server.URL,
			expectError: false,
		},
		{
			name:        "invalid server URL",
			args:        []string{"--server", "invalid://url"},
			serverURL:   "invalid://url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Commands = []cli.Command{
				{
					Name:   "list-plugins",
					Action: cmdListPlugins,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "server",
							Value: "http://localhost:8080",
							Usage: "Server URL",
						},
					},
				},
			}

			err := app.Run(append([]string{"gpud", "list-plugins"}, tt.args...))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCmdListPluginsServerResponses(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/components/custom-plugin" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// format := r.URL.Query().Get("format")
		// accept := r.Header.Get("Accept")

		// Determine which test case is being run based on Accept header or query param
		// if accept == "application/json" || format == "json" {
		// 	w.Header().Set("Content-Type", "application/json")
		// 	w.WriteHeader(http.StatusOK)
		// 	if _, err := w.Write([]byte(`{"plugin1":{"type":"shell","run_mode":"async"},"plugin2":{"type":"python","run_mode":"sync"}}`)); err != nil {
		// 		t.Logf("Error writing response: %v", err)
		// 	}
		// 	return
		// }
		// if accept == "application/yaml" || format == "yaml" {
		// 	w.Header().Set("Content-Type", "application/yaml")
		// 	w.WriteHeader(http.StatusOK)
		// 	if _, err := w.Write([]byte(`plugin1:\n  type: shell\n  run_mode: async\nplugin2:\n  type: python\n  run_mode: sync`)); err != nil {
		// 		t.Logf("Error writing response: %v", err)
		// 	}
		// 	return
		// }
		// For empty response case
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Logf("Error writing response: %v", err)
		}
	}))
	defer server.Close()

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		// 		{
		// 			name:        "json response",
		// 			args:        []string{"--server", server.URL, "--format", "json"},
		// 			expectError: false,
		// 		},
		// 		{
		// 			name:        "yaml response",
		// 			args:        []string{"--server", server.URL, "--format", "yaml"},
		// 			expectError: false,
		// 		},
		// 		{
		// 			name:        "empty response",
		// 			args:        []string{"--server", server.URL},
		// 			expectError: false,
		// 		},
		{
			name:        "server error",
			args:        []string{"--server", server.URL + "/invalid"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Commands = []cli.Command{
				{
					Name:   "list-plugins",
					Action: cmdListPlugins,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "server",
							Value: "http://localhost:8080",
							Usage: "Server URL",
						},
						cli.StringFlag{
							Name:  "format",
							Value: "json",
							Usage: "Response format (json/yaml)",
						},
					},
				},
			}

			err := app.Run(append([]string{"gpud", "list-plugins"}, tt.args...))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
