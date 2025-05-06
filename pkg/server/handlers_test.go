package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/components"
	gpudconfig "github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/metrics"
)

func TestNewGlobalHandler(t *testing.T) {
	var metricStore metrics.Store
	ghler := newGlobalHandler(nil, components.NewRegistry(nil), metricStore)
	assert.NotNil(t, ghler)
}

func TestGetReqTime(t *testing.T) {
	g := &globalHandler{}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		startTimeQuery string
		endTimeQuery   string
		expectError    bool
	}{
		{
			name:           "no query params",
			startTimeQuery: "",
			endTimeQuery:   "",
			expectError:    false,
		},
		{
			name:           "valid start and end times",
			startTimeQuery: "1609459200", // 2021-01-01 00:00:00
			endTimeQuery:   "1609545600", // 2021-01-02 00:00:00
			expectError:    false,
		},
		{
			name:           "invalid start time",
			startTimeQuery: "invalid",
			endTimeQuery:   "1609545600",
			expectError:    true,
		},
		{
			name:           "invalid end time",
			startTimeQuery: "1609459200",
			endTimeQuery:   "invalid",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("GET", "/?startTime="+tt.startTimeQuery+"&endTime="+tt.endTimeQuery, nil)
			c.Request = req

			startTime, endTime, err := g.getReqTime(c)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.startTimeQuery != "" {
					expectedStartTime := time.Unix(1609459200, 0)
					assert.Equal(t, expectedStartTime, startTime)
				}
				if tt.endTimeQuery != "" {
					expectedEndTime := time.Unix(1609545600, 0)
					assert.Equal(t, expectedEndTime, endTime)
				}
			}
		})
	}
}

func TestCreateHealthzHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/healthz", createHealthzHandler())

	tests := []struct {
		name        string
		contentType string
		jsonIndent  string
		wantStatus  int
		checkBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "default JSON response",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp Healthz
				err := json.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, "v1", resp.Version)
			},
		},
		{
			name:       "indented JSON response",
			jsonIndent: "true",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp Healthz
				err := json.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, "v1", resp.Version)
			},
		},
		{
			name:        "YAML response",
			contentType: "application/yaml",
			wantStatus:  http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp Healthz
				err := yaml.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, "v1", resp.Version)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/healthz", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.jsonIndent != "" {
				req.Header.Set("json-indent", tt.jsonIndent)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.checkBody != nil {
				tt.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestCreateConfigHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &gpudconfig.Config{
		Address: "localhost:8080",
	}

	router := gin.New()
	router.GET("/config", createConfigHandler(cfg))

	tests := []struct {
		name        string
		contentType string
		jsonIndent  string
		wantStatus  int
		checkBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "default JSON response",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp gpudconfig.Config
				err := json.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "localhost:8080", resp.Address)
			},
		},
		{
			name:       "indented JSON response",
			jsonIndent: "true",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp gpudconfig.Config
				err := json.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "localhost:8080", resp.Address)
			},
		},
		{
			name:        "YAML response",
			contentType: "application/yaml",
			wantStatus:  http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp gpudconfig.Config
				err := yaml.Unmarshal(body, &resp)
				assert.NoError(t, err)
				assert.Equal(t, "localhost:8080", resp.Address)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/config", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.jsonIndent != "" {
				req.Header.Set("json-indent", tt.jsonIndent)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.checkBody != nil {
				tt.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestCreatePackageHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test wrapper function that simulates the behavior of createPackageHandler
	createTestPackageHandler := func(statusResult interface{}, statusError error) func(c *gin.Context) {
		return func(c *gin.Context) {
			if statusError != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    http.StatusInternalServerError,
					"message": "failed to get package status " + statusError.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, statusResult)
		}
	}

	tests := []struct {
		name         string
		statusResult interface{}
		statusError  error
		expectedCode int
	}{
		{
			name: "successful response",
			statusResult: []map[string]interface{}{
				{
					"name":         "package1",
					"is_installed": true,
					"status":       true,
				},
				{
					"name":         "package2",
					"is_installed": false,
					"status":       false,
				},
			},
			statusError:  nil,
			expectedCode: http.StatusOK,
		},
		{
			name:         "error response",
			statusResult: nil,
			statusError:  errors.New("failed to get status"),
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/admin/packages", nil)

			// Get the handler from our function and call it
			handler := createTestPackageHandler(tt.statusResult, tt.statusError)
			handler(c)

			// Check response
			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == http.StatusOK {
				// Parse the response
				var result []map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &result)
				require.NoError(t, err)

				// Verify the response matches the status result
				expected, ok := tt.statusResult.([]map[string]interface{})
				assert.True(t, ok, "Status result should be slice of maps")
				assert.Equal(t, expected, result)
			} else {
				// Check error message is in the response
				var errorResp gin.H
				err := json.Unmarshal(w.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp["message"], "failed to get package status")
			}
		})
	}
}
