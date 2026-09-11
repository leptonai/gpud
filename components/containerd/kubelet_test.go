package containerd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKubeletReadOnlyPodsJSON = `{
  "kind": "PodList",
  "apiVersion": "v1",
  "metadata": {},
  "items": [
    {
      "metadata": {
        "name": "vector-jldbs",
        "namespace": "default",
        "uid": "1d4252aa-a751-4b10-9c68-5b1396b9c999"
      },
      "spec": {
        "nodeName": "mynodehostname"
      },
      "status": {
        "phase": "Running"
      }
    },
    {
      "metadata": {
        "name": "kube-proxy-hfqwt",
        "namespace": "kube-system",
        "uid": "2d4252aa-a751-4b10-9c68-5b1396b9c998"
      },
      "spec": {
        "nodeName": "mynodehostname"
      },
      "status": {
        "phase": "Running"
      }
    }
  ]
}`

func newTestKubeletServer(t *testing.T, handler http.HandlerFunc) int {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, err := strconv.ParseInt(portRaw, 10, 32)
	require.NoError(t, err)
	return int(port)
}

func TestListPodsFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	port := newTestKubeletServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/pods", r.URL.Path, "expected path to be '/pods'")
		assert.Equal(t, http.MethodGet, r.Method, "expected GET request")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletReadOnlyPodsJSON))
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletReadOnlyPort(ctx, port)
	require.NoError(t, err)
	require.Len(t, pods, 2, "expected 2 pods")

	assert.Equal(t, "vector-jldbs", pods[0].Name)
	assert.Equal(t, "default", pods[0].Namespace)
	assert.Equal(t, "kube-proxy-hfqwt", pods[1].Name)
	assert.Equal(t, "kube-system", pods[1].Namespace)
}

func TestListPodsFromKubeletReadOnlyPort_ParseError(t *testing.T) {
	t.Parallel()

	port := newTestKubeletServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletReadOnlyPort(ctx, port)

	require.Error(t, err, "expected an error")
	require.Nil(t, pods, "pods should be nil")
}

func TestListPodsFromKubeletReadOnlyPort_ConnError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, err := strconv.ParseInt(portRaw, 10, 32)
	require.NoError(t, err)

	srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))

	require.Error(t, err, "expected an error")
	assert.Nil(t, pods, "pods should be nil")
}

func TestParsePodsFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	pods, err := parsePodsFromKubeletReadOnlyPort(strings.NewReader(testKubeletReadOnlyPodsJSON))
	require.NoError(t, err)
	require.Len(t, pods.Items, 2)
	assert.Equal(t, "vector-jldbs", pods.Items[0].Name)
	assert.Equal(t, "mynodehostname", pods.Items[0].Spec.NodeName)

	_, err = parsePodsFromKubeletReadOnlyPort(strings.NewReader("not a json"))
	require.Error(t, err)
}
