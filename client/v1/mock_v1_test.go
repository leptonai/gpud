package v1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/httputil"
)

// errorTransport implements http.RoundTripper and always returns an error.
type errorTransport struct {
	err error
}

func (t *errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

// --- HTTP Client Do() Error Tests ---

func TestGetComponents_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetComponents with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetComponents(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestGetInfo_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetInfo with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetInfo(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestGetHealthStates_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetHealthStates with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetHealthStates(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestGetEvents_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetEvents with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetEvents(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestGetMetrics_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetMetrics with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetMetrics(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestGetPluginSpecs_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("GetPluginSpecs with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := GetPluginSpecs(context.Background(), "http://localhost:8080")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestDeregisterComponent_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("DeregisterComponent with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		err := DeregisterComponent(context.Background(), "http://localhost:8080", "test-comp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
	})
}

func TestTriggerComponent_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("TriggerComponent with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := TriggerComponent(context.Background(), "http://localhost:8080", "test-comp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

func TestTriggerComponentCheckByTag_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("TriggerComponentCheckByTag with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		err := TriggerComponentCheckByTag(context.Background(), "http://localhost:8080", "test-tag")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
	})
}

func TestSetHealthyComponents_HTTPClientError(t *testing.T) {
	mockey.PatchConvey("SetHealthyComponents with HTTP client error", t, func() {
		mockey.Mock(createDefaultHTTPClient).To(func() *http.Client {
			return &http.Client{Transport: &errorTransport{err: errors.New("connection refused")}}
		}).Build()
		result, err := SetHealthyComponents(context.Background(), "http://localhost:8080", []string{"disk"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to make request")
		assert.Nil(t, result)
	})
}

// --- URL Parse / Request Creation Error Tests ---

func TestGetComponents_RequestCreationError(t *testing.T) {
	_, err := GetComponents(context.Background(), "http://[invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestGetEvents_RequestCreationError(t *testing.T) {
	_, err := GetEvents(context.Background(), "http://[invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestGetMetrics_RequestCreationError(t *testing.T) {
	_, err := GetMetrics(context.Background(), "http://[invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestGetInfo_URLParseError(t *testing.T) {
	_, err := GetInfo(context.Background(), "http://[invalid")
	require.Error(t, err)
}

func TestGetHealthStates_URLParseError(t *testing.T) {
	_, err := GetHealthStates(context.Background(), "http://[invalid")
	require.Error(t, err)
}

func TestGetPluginSpecs_URLParseError(t *testing.T) {
	_, err := GetPluginSpecs(context.Background(), "http://[invalid")
	require.Error(t, err)
}

func TestDeregisterComponent_URLParseError(t *testing.T) {
	err := DeregisterComponent(context.Background(), "http://[invalid", "comp")
	require.Error(t, err)
}

func TestTriggerComponent_URLParseError(t *testing.T) {
	_, err := TriggerComponent(context.Background(), "http://[invalid", "comp")
	require.Error(t, err)
}

func TestTriggerComponentCheckByTag_URLParseError(t *testing.T) {
	err := TriggerComponentCheckByTag(context.Background(), "http://[invalid", "tag")
	require.Error(t, err)
}

func TestSetHealthyComponents_URLParseError2(t *testing.T) {
	_, err := SetHealthyComponents(context.Background(), "http://[invalid", []string{"disk"})
	require.Error(t, err)
}

// --- io.ReadAll Error Tests (mocking for YAML read paths) ---

func TestReadComponents_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- comp1\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadComponents(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadComponents(bytes.NewReader([]byte("- comp1\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

func TestReadInfo_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- component: comp1\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadInfo(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadInfo(bytes.NewReader([]byte("- component: comp1\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

func TestReadHealthStates_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- component: comp1\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadHealthStates(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadHealthStates(bytes.NewReader([]byte("- component: comp1\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

func TestReadEvents_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- component: comp1\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadEvents(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadEvents(bytes.NewReader([]byte("- component: comp1\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

func TestReadMetrics_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- component: comp1\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadMetrics(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadMetrics(bytes.NewReader([]byte("- component: comp1\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

func TestReadPluginSpecs_YAMLReadAllErrors(t *testing.T) {
	t.Run("gzip YAML ReadAll error", func(t *testing.T) {
		gzippedData := gzipContent(t, []byte("- pluginName: test\n"))
		mockey.PatchConvey("gzip YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadPluginSpecs(bytes.NewReader(gzippedData), WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})

	t.Run("plain YAML ReadAll error", func(t *testing.T) {
		mockey.PatchConvey("plain YAML ReadAll error", t, func() {
			mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read error")
			}).Build()
			_, err := ReadPluginSpecs(bytes.NewReader([]byte("- pluginName: test\n")), WithRequestContentTypeYAML())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read yaml")
		})
	})
}

// --- DeregisterComponent response body read error ---

func TestDeregisterComponent_ReadResponseBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	mockey.PatchConvey("DeregisterComponent io.ReadAll error on response body", t, func() {
		mockey.Mock(io.ReadAll).To(func(r io.Reader) ([]byte, error) {
			return nil, errors.New("read error")
		}).Build()
		err := DeregisterComponent(context.Background(), srv.URL, "test-comp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response body")
	})
}

// --- DeregisterComponent with accept encoding header ---

func TestDeregisterComponent_WithAcceptEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/components", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, httputil.RequestHeaderEncodingGzip, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	err := DeregisterComponent(context.Background(), srv.URL, "test-comp",
		WithRequestContentTypeJSON(), WithAcceptEncodingGzip())
	require.NoError(t, err)
}

// --- cloneStringSlice edge cases ---

func TestCloneStringSlice_NilAndEmpty(t *testing.T) {
	result := cloneStringSlice(nil)
	assert.Nil(t, result)

	result = cloneStringSlice([]string{})
	assert.Nil(t, result)
}
