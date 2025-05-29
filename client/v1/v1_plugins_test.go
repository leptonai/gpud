package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestReadCustomPluginSpecs_Comprehensive(t *testing.T) {
	testPlugins := pkgcustomplugins.Specs{
		{
			PluginName: "test",
			PluginType: pkgcustomplugins.SpecTypeComponent,
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							Script:      "echo hello",
							ContentType: "plaintext",
						},
					},
				},
			},
			Timeout: metav1.Duration{Duration: time.Minute},
		},
	}
	jsonData := mustMarshalJSON(t, testPlugins)
	yamlData := mustMarshalYAML(t, testPlugins)

	gzipJSON := func() []byte {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(jsonData)
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		return buf.Bytes()
	}()

	gzipYAML := func() []byte {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(yamlData)
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		return buf.Bytes()
	}()

	tests := []struct {
		name           string
		input          io.Reader
		contentType    string
		acceptEncoding string
		expectedError  string
		expectedResult pkgcustomplugins.Specs
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testPlugins,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testPlugins,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipJSON),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipYAML),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   httputil.RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   httputil.RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to create gzip reader",
		},
		{
			name:          "unsupported content type",
			input:         bytes.NewReader(jsonData),
			contentType:   "application/xml",
			expectedError: "unsupported content type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []OpOption{}
			if tt.contentType != "" {
				if tt.contentType == httputil.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == httputil.RequestHeaderJSON {
					opts = append(opts, WithRequestContentTypeJSON())
				} else {
					opts = append(opts, func(op *Op) {
						op.requestContentType = tt.contentType
					})
				}
			}
			if tt.acceptEncoding != "" {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := ReadPluginSpecs(tt.input, opts...)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetCustomPlugins(t *testing.T) {
	testPlugins := pkgcustomplugins.Specs{
		{
			PluginName: "test",
			PluginType: pkgcustomplugins.SpecTypeComponent,
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
					},
				},
			},
			Timeout: metav1.Duration{Duration: time.Minute},
		},
	}

	tests := []struct {
		name           string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult pkgcustomplugins.Specs
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testPlugins),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testPlugins),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: mustMarshalJSON(t, testPlugins),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
			useGzip:        true,
		},
		{
			name:           "not found error",
			serverResponse: []byte(`not found`),
			statusCode:     http.StatusNotFound,
			expectedError:  errdefs.ErrNotFound.Error(),
		},
		{
			name:           "server error",
			serverResponse: []byte(`internal error`),
			statusCode:     http.StatusInternalServerError,
			expectedError:  "server not ready, response not 200",
		},
		{
			name:           "empty response",
			serverResponse: []byte(`[]`),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: pkgcustomplugins.Specs{},
		},
		{
			name:           "invalid JSON response",
			serverResponse: []byte(`invalid json`),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/plugins", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				if tt.useGzip {
					_, err := w.Write(gzipContent(t, tt.serverResponse))
					require.NoError(t, err)
				} else {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := GetPluginSpecs(context.Background(), srv.URL, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
