package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"

	gpudconfig "github.com/leptonai/gpud/pkg/config"
)

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
