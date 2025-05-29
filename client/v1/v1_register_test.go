package v1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestRegisterOrUpdateCustomPlugin(t *testing.T) {
	validSpec := pkgcustomplugins.Spec{
		PluginName: "test-plugin",
		Type:       pkgcustomplugins.SpecTypeComponent,
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					RunBashScript: &pkgcustomplugins.RunBashScript{
						Script:      "echo 'hello'",
						ContentType: "plaintext",
					},
					Name: "test-step",
				},
			},
		},
		Timeout: metav1.Duration{Duration: time.Minute},
	}

	tests := []struct {
		name           string
		method         string
		spec           pkgcustomplugins.Spec
		contentType    string
		acceptEncoding string
		serverResp     string
		statusCode     int
		expectedError  string
		validateOnly   bool
	}{
		{
			name:        "register with POST method",
			method:      http.MethodPost,
			spec:        validSpec,
			contentType: httputil.RequestHeaderJSON,
			serverResp:  "ok",
			statusCode:  http.StatusOK,
		},
		{
			name:        "update with PUT method",
			method:      http.MethodPut,
			spec:        validSpec,
			contentType: httputil.RequestHeaderJSON,
			serverResp:  "ok",
			statusCode:  http.StatusOK,
		},
		{
			name:           "with YAML content type",
			method:         http.MethodPost,
			spec:           validSpec,
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			serverResp:     "ok",
			statusCode:     http.StatusOK,
		},
		{
			name:          "invalid spec",
			method:        http.MethodPost,
			spec:          pkgcustomplugins.Spec{}, // Missing required fields
			expectedError: "invalid spec",
			validateOnly:  true,
		},
		{
			name:          "server error",
			method:        http.MethodPost,
			spec:          validSpec,
			contentType:   httputil.RequestHeaderJSON,
			statusCode:    http.StatusInternalServerError,
			expectedError: "server not ready, response not 200",
		},
		{
			name:          "unsupported method",
			method:        http.MethodDelete,
			spec:          validSpec,
			contentType:   httputil.RequestHeaderJSON,
			expectedError: "unsupported method",
			validateOnly:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.validateOnly {
				// Test validation error without creating a server
				err := registerOrUpdateCustomPlugin(context.Background(), "http://localhost:8080", tt.spec, tt.method)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/components/custom-plugin", r.URL.Path)
				assert.Equal(t, tt.method, r.Method)

				// Verify request headers
				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
				}

				// Verify the request body contains the correct spec
				var receivedSpec pkgcustomplugins.Spec
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				err = json.Unmarshal(body, &receivedSpec)
				require.NoError(t, err)
				assert.Equal(t, tt.spec.PluginName, receivedSpec.PluginName)
				assert.Equal(t, tt.spec.Type, receivedSpec.Type)

				w.WriteHeader(tt.statusCode)
				_, err = w.Write([]byte(tt.serverResp))
				require.NoError(t, err)
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding != "" {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			err := registerOrUpdateCustomPlugin(context.Background(), srv.URL, tt.spec, tt.method, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
		})
	}
}
