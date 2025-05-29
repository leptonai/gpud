package listplugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
)

func TestCmdListPlugins(t *testing.T) {
	// Create a test server that returns an empty array (no plugins)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/plugins" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`[]`)); err != nil {
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
					Action: Command,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "server",
							Value: "http://localhost:8080",
							Usage: "Server URL",
						},
						cli.StringFlag{
							Name:  "log-level",
							Value: "info",
							Usage: "Log level",
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
	// Create test plugin specs that match the actual structure
	testPlugins := pkgcustomplugins.Specs{
		{
			PluginName: "test-plugin-1",
			Type:       pkgcustomplugins.SpecTypeComponent,
			RunMode:    "auto",
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo hello",
						},
					},
				},
			},
			Timeout: metav1.Duration{Duration: time.Minute},
		},
		{
			PluginName: "test-plugin-2",
			Type:       pkgcustomplugins.SpecTypeComponent,
			RunMode:    "manual",
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step-2",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo world",
						},
					},
				},
			},
			Timeout: metav1.Duration{Duration: 30 * time.Second},
		},
	}

	// Marshal test plugins to JSON
	testPluginsJSON, err := json.Marshal(testPlugins)
	if err != nil {
		t.Fatalf("Failed to marshal test plugins: %v", err)
	}

	tests := []struct {
		name        string
		serverFunc  func(w http.ResponseWriter, r *http.Request)
		args        []string
		expectError bool
	}{
		{
			name: "successful response with plugins",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/plugins" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write(testPluginsJSON); err != nil {
					t.Logf("Error writing response: %v", err)
				}
			},
			expectError: false,
		},
		{
			name: "empty response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/plugins" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`[]`)); err != nil {
					t.Logf("Error writing response: %v", err)
				}
			},
			expectError: false,
		},
		{
			name: "server error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a separate server for each test case
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			args := []string{"--server", server.URL}
			if tt.name == "server error" {
				// For server error test, use an invalid path
				args = []string{"--server", server.URL + "/invalid"}
			}

			app := cli.NewApp()
			app.Commands = []cli.Command{
				{
					Name:   "list-plugins",
					Action: Command,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "server",
							Value: "http://localhost:8080",
							Usage: "Server URL",
						},
						cli.StringFlag{
							Name:  "log-level",
							Value: "info",
							Usage: "Log level",
						},
					},
				},
			}

			err := app.Run(append([]string{"gpud", "list-plugins"}, args...))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
