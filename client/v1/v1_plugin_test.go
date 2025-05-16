package v1

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
)

func TestReadCustomPluginSpecs_Comprehensive(t *testing.T) {
	testPlugins := map[string]pkgcustomplugins.Spec{
		"test": {
			PluginName: "test",
			Type:       pkgcustomplugins.SpecTypeComponent,
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
		expectedResult map[string]pkgcustomplugins.Spec
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testPlugins,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    RequestHeaderJSON,
			expectedResult: testPlugins,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    RequestHeaderYAML,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipJSON),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipYAML),
			contentType:    RequestHeaderYAML,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
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
				if tt.contentType == RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == RequestHeaderJSON {
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

			result, err := ReadCustomPluginSpecs(tt.input, opts...)
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
