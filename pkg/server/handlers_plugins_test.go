package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestGetPluginSpecs(t *testing.T) {
	// Add a regular component
	regularComp := &mockComponent{
		name:           "regular-comp",
		isSupported:    true,
		isCustomPlugin: false,
	}

	// Add a custom plugin component with a valid Spec
	spec := pkgcustomplugins.Spec{
		PluginName: "custom-plugin",
		PluginType: pkgcustomplugins.SpecTypeComponent,
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
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	customComp := &mockComponent{
		name:           "custom-plugin",
		isSupported:    true,
		isCustomPlugin: true,
		spec:           spec,
	}

	// Setup handler with both components
	handler, _, _ := setupTestHandler([]components.Component{regularComp, customComp})
	_, c, w := setupTestRouter()

	// Set up request for the handler
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response - expecting an array, not a map
	var plugins pkgcustomplugins.Specs
	err := json.Unmarshal(w.Body.Bytes(), &plugins)
	require.NoError(t, err)

	// Only the custom plugin should be in the response
	assert.Len(t, plugins, 1)
	assert.Equal(t, "custom-plugin", plugins[0].PluginName)
	assert.Equal(t, pkgcustomplugins.SpecTypeComponent, plugins[0].PluginType)
}

func TestGetPluginSpecsYAML(t *testing.T) {
	// Add a custom plugin component
	spec := pkgcustomplugins.Spec{
		PluginName: "yaml-plugin",
		PluginType: pkgcustomplugins.SpecTypeComponent,
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "yaml-step",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo yaml",
					},
				},
			},
		},
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	customComp := &mockComponent{
		name:           "yaml-plugin",
		isSupported:    true,
		isCustomPlugin: true,
		spec:           spec,
	}

	// Setup handler
	handler, _, _ := setupTestHandler([]components.Component{customComp})
	_, c, w := setupTestRouter()

	// Set up request with YAML content type
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the YAML response
	var plugins pkgcustomplugins.Specs
	err := yaml.Unmarshal(w.Body.Bytes(), &plugins)
	require.NoError(t, err)

	assert.Len(t, plugins, 1)
	assert.Equal(t, "yaml-plugin", plugins[0].PluginName)
}

func TestGetPluginSpecsIndentedJSON(t *testing.T) {
	// Add a custom plugin component
	spec := pkgcustomplugins.Spec{
		PluginName: "indent-plugin",
		PluginType: pkgcustomplugins.SpecTypeComponent,
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "indent-step",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo indent",
					},
				},
			},
		},
		Timeout: metav1.Duration{Duration: 15 * time.Second},
	}

	customComp := &mockComponent{
		name:           "indent-plugin",
		isSupported:    true,
		isCustomPlugin: true,
		spec:           spec,
	}

	// Setup handler
	handler, _, _ := setupTestHandler([]components.Component{customComp})
	_, c, w := setupTestRouter()

	// Set up request with JSON indent header
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
	c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Check that the response is indented (contains newlines and spaces)
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "\n")
	assert.Contains(t, responseBody, "  ") // Indentation spaces

	// Parse the response
	var plugins pkgcustomplugins.Specs
	err := json.Unmarshal(w.Body.Bytes(), &plugins)
	require.NoError(t, err)

	assert.Len(t, plugins, 1)
	assert.Equal(t, "indent-plugin", plugins[0].PluginName)
}

func TestGetPluginSpecsInvalidContentType(t *testing.T) {
	// Setup handler with no components
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	// Set up request with invalid content type
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "invalid content type", response["message"])
}

func TestGetPluginSpecsNoCustomPlugins(t *testing.T) {
	// Add only regular components (no custom plugins)
	regularComp1 := &mockComponent{
		name:           "regular-comp-1",
		isSupported:    true,
		isCustomPlugin: false,
	}

	regularComp2 := &mockComponent{
		name:           "regular-comp-2",
		isSupported:    true,
		isCustomPlugin: false,
	}

	// Setup handler with only regular components
	handler, _, _ := setupTestHandler([]components.Component{regularComp1, regularComp2})
	_, c, w := setupTestRouter()

	// Set up request
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response - should be empty array
	var plugins pkgcustomplugins.Specs
	err := json.Unmarshal(w.Body.Bytes(), &plugins)
	require.NoError(t, err)

	assert.Len(t, plugins, 0)
}

func TestGetPluginSpecsMultipleCustomPlugins(t *testing.T) {
	// Add multiple custom plugin components
	spec1 := pkgcustomplugins.Spec{
		PluginName: "plugin-1",
		PluginType: pkgcustomplugins.SpecTypeComponent,
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "step-1",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo plugin1",
					},
				},
			},
		},
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	spec2 := pkgcustomplugins.Spec{
		PluginName: "plugin-2",
		PluginType: pkgcustomplugins.SpecTypeComponent,
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "step-2",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo plugin2",
					},
				},
			},
		},
		Timeout: metav1.Duration{Duration: 20 * time.Second},
	}

	customComp1 := &mockComponent{
		name:           "plugin-1",
		isSupported:    true,
		isCustomPlugin: true,
		spec:           spec1,
	}

	customComp2 := &mockComponent{
		name:           "plugin-2",
		isSupported:    true,
		isCustomPlugin: true,
		spec:           spec2,
	}

	// Setup handler with multiple custom plugins
	handler, _, _ := setupTestHandler([]components.Component{customComp1, customComp2})
	_, c, w := setupTestRouter()

	// Set up request
	c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)

	// Call the handler
	handler.getPluginSpecs(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var plugins pkgcustomplugins.Specs
	err := json.Unmarshal(w.Body.Bytes(), &plugins)
	require.NoError(t, err)

	assert.Len(t, plugins, 2)

	// Check that both plugins are present
	pluginNames := make([]string, len(plugins))
	for i, plugin := range plugins {
		pluginNames[i] = plugin.PluginName
	}
	assert.Contains(t, pluginNames, "plugin-1")
	assert.Contains(t, pluginNames, "plugin-2")
}

func TestRegisterPluginRoutes(t *testing.T) {
	// Setup handler
	handler, _, _ := setupTestHandler(nil)

	// Setup router with "/v1" path
	router, v1 := setupRouterWithPath("/v1")

	// Register routes
	handler.registerPluginRoutes(v1)

	// Create a test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Test the plugins endpoint
	resp, err := http.Get(server.URL + "/v1/plugins")
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	// Should get a response (we don't care about the exact content, just that the route is registered)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
