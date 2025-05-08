package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"
)

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
