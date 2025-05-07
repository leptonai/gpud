package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

func TestTriggerComponentsByTag(t *testing.T) {
	// Create a test instance with some components
	instance := &components.GPUdInstance{
		RootCtx: context.Background(),
	}
	registry := components.NewRegistry(instance)

	// Register test components with different tags
	comp1 := &testComponent{name: "comp1", tags: []string{"tag1", "tag2"}}
	comp2 := &testComponent{name: "comp2", tags: []string{"tag2", "tag3"}}
	comp3 := &testComponent{name: "comp3", tags: []string{"tag3", "tag4"}}

	registry.Register(func(*components.GPUdInstance) (components.Component, error) { return comp1, nil })
	registry.Register(func(*components.GPUdInstance) (components.Component, error) { return comp2, nil })
	registry.Register(func(*components.GPUdInstance) (components.Component, error) { return comp3, nil })

	// Create a test handler
	handler := &globalHandler{
		componentsRegistry: registry,
	}

	tests := []struct {
		name           string
		tagName        string
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "trigger components with tag1",
			tagName:        "tag1",
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"components": []string{"comp1"},
				"exit":       0,
			},
		},
		{
			name:           "trigger components with tag2",
			tagName:        "tag2",
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"components": []string{"comp1", "comp2"},
				"exit":       0,
			},
		},
		{
			name:           "trigger components with tag3",
			tagName:        "tag3",
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"components": []string{"comp2", "comp3"},
				"exit":       0,
			},
		},
		{
			name:           "trigger components with non-existent tag",
			tagName:        "nonexistent",
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"components": []string{},
				"exit":       0,
			},
		},
		{
			name:           "missing tag parameter",
			tagName:        "",
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"error": "tagName parameter is required",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request
			req := httptest.NewRequest("GET", "/v1/components/trigger-tag", nil)
			if tt.tagName != "" {
				q := req.URL.Query()
				q.Add("tagName", tt.tagName)
				req.URL.RawQuery = q.Encode()
			}

			// Create a response recorder
			w := httptest.NewRecorder()

			// Create a gin context
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Call the handler
			handler.triggerComponentsByTag(c)

			// Check the response
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				// Check components list
				components, ok := response["components"].([]interface{})
				assert.True(t, ok)
				componentNames := make([]string, len(components))
				for i, c := range components {
					componentNames[i] = c.(string)
				}
				assert.ElementsMatch(t, tt.expectedBody["components"].([]string), componentNames)

				// Check exit status
				assert.Equal(t, tt.expectedBody["exit"], response["exit"])
			} else {
				// Check error message
				assert.Equal(t, tt.expectedBody["error"], response["error"])
			}
		})
	}
}

// testComponent is a simple implementation of the Component interface for testing
type testComponent struct {
	name string
	tags []string
}

func (c *testComponent) Name() string {
	return c.name
}

func (c *testComponent) Tags() []string {
	return c.tags
}

func (c *testComponent) Start() error {
	return nil
}

func (c *testComponent) Check() components.CheckResult {
	return &testCheckResult{componentName: c.name}
}

func (c *testComponent) LastHealthStates() v1.HealthStates {
	return nil
}

func (c *testComponent) Events(ctx context.Context, since time.Time) (v1.Events, error) {
	return nil, nil
}

func (c *testComponent) Close() error {
	return nil
}

// testCheckResult is a simple implementation of the CheckResult interface for testing
type testCheckResult struct {
	componentName string
}

func (cr *testCheckResult) ComponentName() string {
	return cr.componentName
}

func (cr *testCheckResult) String() string {
	return "test check result"
}

func (cr *testCheckResult) Summary() string {
	return "test summary"
}

func (cr *testCheckResult) HealthStateType() v1.HealthStateType {
	return v1.HealthStateTypeHealthy
}

func (cr *testCheckResult) HealthStates() v1.HealthStates {
	return v1.HealthStates{
		{
			Time:   metav1.Now(),
			Health: v1.HealthStateTypeHealthy,
			Reason: "test reason",
			Error:  "test error",
		},
	}
}
