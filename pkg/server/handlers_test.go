package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
