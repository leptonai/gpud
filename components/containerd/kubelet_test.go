package containerd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKubeletPodsJSON = `{
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
        "nodeName": "test-node"
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
        "nodeName": "test-node"
      },
      "status": {
        "phase": "Running"
      }
    }
  ]
}`

const testKubeletEmptyPodsJSON = `{
  "kind": "PodList",
  "apiVersion": "v1",
  "metadata": {},
  "items": []
}`

// testPKI holds a self-signed CA, a server certificate, and a combined
// client certificate+key PEM (the same shape as kubelet-client-current.pem).
type testPKI struct {
	caPool        *x509.CertPool
	caPEM         []byte
	serverCert    tls.Certificate
	clientCertPEM []byte
}

// newTestPKI creates a CA, a server certificate with the given SANs, and a
// client certificate with the given CN, both signed by that CA.
func newTestPKI(t *testing.T, serverDNS []string, serverIPs []net.IP, clientCN string) testPKI {
	t.Helper()

	now := time.Now()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	newSignedCert := func(template *x509.Certificate) (tls.Certificate, []byte) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
		require.NoError(t, err)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, err := x509.MarshalECPrivateKey(key)
		require.NoError(t, err)
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		require.NoError(t, err)
		return cert, append(append([]byte{}, certPEM...), keyPEM...)
	}

	serverCert, _ := newSignedCert(&x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "kubelet"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     serverDNS,
		IPAddresses:  serverIPs,
	})
	_, clientCertKeyPEM := newSignedCert(&x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: clientCN},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	return testPKI{
		caPool:        caPool,
		caPEM:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		serverCert:    serverCert,
		clientCertPEM: clientCertKeyPEM,
	}
}

// newTestAPIServer starts a TLS test server presenting the PKI's server
// certificate, emulating the API server endpoint kubelet talks to. When
// requireClientCert is set, the server requires a client certificate signed by
// the PKI's CA, mirroring webhook-authenticated kubelet API access.
func newTestAPIServer(t *testing.T, pki testPKI, requireClientCert bool, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverCert},
	}
	if requireClientCert {
		srv.TLS.ClientCAs = pki.caPool
		srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// writeKubeletCredentials writes the kubelet kubeconfig, cluster CA, and
// combined client cert/key PEM into a temp dir, mirroring the on-host layout
// (/root/.kube/config, /etc/kubernetes/pki/ca.crt,
// /etc/kubernetes/pki/kubelet-client-current.pem).
func writeKubeletCredentials(t *testing.T, pki testPKI, serverURL string, tlsServerName string) (kubeconfigPath, clientCertPath, caPath string) {
	t.Helper()
	dir := t.TempDir()

	tlsServerNameLine := ""
	if tlsServerName != "" {
		tlsServerNameLine = fmt.Sprintf("    tls-server-name: %s\n", tlsServerName)
	}
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: kubelet
contexts:
- name: kubelet
  context:
    cluster: test-cluster
    user: kubelet
clusters:
- name: test-cluster
  cluster:
    server: %s
%susers:
- name: kubelet
  user: {}
`, serverURL, tlsServerNameLine)

	kubeconfigPath = filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0600))

	caPath = filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caPath, pki.caPEM, 0644))

	clientCertPath = filepath.Join(dir, "kubelet-client-current.pem")
	require.NoError(t, os.WriteFile(clientCertPath, pki.clientCertPEM, 0600))

	return kubeconfigPath, clientCertPath, caPath
}

// newTestKubeletAPIConfig loads the kubelet API config from credentials
// written to a temp dir, exercising the same loader the component uses.
func newTestKubeletAPIConfig(t *testing.T, pki testPKI, serverURL string, tlsServerName string) *kubeletAPIConfig {
	t.Helper()
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, serverURL, tlsServerName)
	cfg, err := loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath)
	require.NoError(t, err)
	return cfg
}

func TestLoadKubeletAPIConfig(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "https://10.0.0.1:6443", "apiserver.internal")

	cfg, err := loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1:6443", cfg.server.Host)
	assert.Equal(t, "apiserver.internal", cfg.serverName)
	assert.Equal(t, "test-node", cfg.nodeName)
	assert.NotNil(t, cfg.caPool)
}

func TestLoadKubeletAPIConfig_MissingFiles(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "https://10.0.0.1:6443", "")

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	for _, tt := range []struct {
		name           string
		kubeconfigPath string
		clientCertPath string
		caPath         string
	}{
		{"missing kubeconfig", missing, clientCertPath, caPath},
		{"missing client cert", kubeconfigPath, missing, caPath},
		{"missing ca", kubeconfigPath, clientCertPath, missing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadKubeletAPIConfig(tt.kubeconfigPath, tt.clientCertPath, tt.caPath)
			require.Error(t, err)
			assert.ErrorIs(t, err, errKubeletIdentityNotFound,
				"missing credential files must map to errKubeletIdentityNotFound")
		})
	}
}

func TestLoadKubeletAPIConfig_NonNodeClientCert(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:admin")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "https://10.0.0.1:6443", "")

	_, err := loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errKubeletIdentityNotFound,
		"an existing but non-node identity must surface as an error, not as a skip")
	assert.Contains(t, err.Error(), systemNodePrefix)
}

func TestLoadKubeletAPIConfig_NonHTTPS(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "http://10.0.0.1:8080", "")

	_, err := loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not https")
}

func TestListPodsUsingKubeletIdentity(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/pods", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "spec.nodeName=test-node", r.URL.Query().Get("fieldSelector"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})

	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, srv.URL, "")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsUsingKubeletIdentity(ctx, kubeconfigPath, clientCertPath, caPath)
	require.NoError(t, err)
	require.Len(t, pods, 2)

	assert.Equal(t, "vector-jldbs", pods[0].Name)
	assert.Equal(t, "default", pods[0].Namespace)
	assert.Equal(t, "kube-proxy-hfqwt", pods[1].Name)
	assert.Equal(t, "kube-system", pods[1].Namespace)
}

func TestListPodsUsingKubeletIdentity_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := listPodsUsingKubeletIdentity(ctx,
		filepath.Join(dir, "kubeconfig"),
		filepath.Join(dir, "kubelet-client-current.pem"),
		filepath.Join(dir, "ca.crt"),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKubeletIdentityNotFound)
}

func TestResolveKubeletIdentityPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := func(name string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0600))
		return p
	}
	missing := filepath.Join(dir, "does-not-exist")

	t.Run("first existing candidate wins per file", func(t *testing.T) {
		t.Parallel()
		// mixed layout: each credential resolves from a different directory
		kubeconfigPath, clientCertPath, caPath, err := resolveKubeletIdentityPaths(
			[]string{missing, existing("kubelet.conf"), existing("kubeconfig")},
			[]string{missing, existing("kubelet-client-current.pem")},
			[]string{existing("ca.crt")},
		)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "kubelet.conf"), kubeconfigPath)
		assert.Equal(t, filepath.Join(dir, "kubelet-client-current.pem"), clientCertPath)
		assert.Equal(t, filepath.Join(dir, "ca.crt"), caPath)
	})

	t.Run("no existing kubeconfig candidate is identity-not-found", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := resolveKubeletIdentityPaths(
			[]string{missing},
			[]string{existing("kubelet-client-current.pem")},
			[]string{existing("ca.crt")},
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, errKubeletIdentityNotFound)
	})

	t.Run("no existing client cert candidate is identity-not-found", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := resolveKubeletIdentityPaths(
			[]string{existing("kubeconfig")},
			[]string{missing},
			[]string{existing("ca.crt")},
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, errKubeletIdentityNotFound)
	})

	t.Run("no existing CA candidate is identity-not-found", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := resolveKubeletIdentityPaths(
			[]string{existing("kubeconfig")},
			[]string{existing("kubelet-client-current.pem")},
			[]string{missing},
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, errKubeletIdentityNotFound)
	})
}

func TestListPodsUsingDiscoveredKubeletIdentity(t *testing.T) {
	// mutates the package-level default candidate paths; do not run in parallel

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})

	// simulate an EKS-style layout under a temp dir: kubelet kubeconfig and
	// rotated client cert under its data dir, CA under the pki dir
	dir := t.TempDir()
	kubeletDir := filepath.Join(dir, "var", "lib", "kubelet")
	pkiDir := filepath.Join(dir, "etc", "kubernetes", "pki")
	require.NoError(t, os.MkdirAll(filepath.Join(kubeletDir, "pki"), 0755))
	require.NoError(t, os.MkdirAll(pkiDir, 0755))

	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, srv.URL, "")
	require.NoError(t, os.Rename(kubeconfigPath, filepath.Join(kubeletDir, "kubeconfig")))
	require.NoError(t, os.Rename(clientCertPath, filepath.Join(kubeletDir, "pki", "kubelet-client-current.pem")))
	require.NoError(t, os.Rename(caPath, filepath.Join(pkiDir, "ca.crt")))

	origKubeconfig, origClientCert, origCA := defaultKubeletKubeconfigPaths, defaultKubeletClientCertPaths, defaultKubeletCAPaths
	t.Cleanup(func() {
		defaultKubeletKubeconfigPaths = origKubeconfig
		defaultKubeletClientCertPaths = origClientCert
		defaultKubeletCAPaths = origCA
	})
	defaultKubeletKubeconfigPaths = []string{filepath.Join(dir, "root", ".kube", "config"), filepath.Join(kubeletDir, "kubeconfig")}
	defaultKubeletClientCertPaths = []string{filepath.Join(pkiDir, "kubelet-client-current.pem"), filepath.Join(kubeletDir, "pki", "kubelet-client-current.pem")}
	defaultKubeletCAPaths = []string{filepath.Join(pkiDir, "ca.crt")}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsUsingDiscoveredKubeletIdentity(ctx)
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, "vector-jldbs", pods[0].Name)
	assert.Equal(t, "kube-proxy-hfqwt", pods[1].Name)
}

func TestListPodsUsingDiscoveredKubeletIdentity_NotFound(t *testing.T) {
	// mutates the package-level default candidate paths; do not run in parallel

	dir := t.TempDir()
	origKubeconfig, origClientCert, origCA := defaultKubeletKubeconfigPaths, defaultKubeletClientCertPaths, defaultKubeletCAPaths
	t.Cleanup(func() {
		defaultKubeletKubeconfigPaths = origKubeconfig
		defaultKubeletClientCertPaths = origClientCert
		defaultKubeletCAPaths = origCA
	})
	defaultKubeletKubeconfigPaths = []string{filepath.Join(dir, "kubeconfig")}
	defaultKubeletClientCertPaths = []string{filepath.Join(dir, "kubelet-client-current.pem")}
	defaultKubeletCAPaths = []string{filepath.Join(dir, "ca.crt")}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := listPodsUsingDiscoveredKubeletIdentity(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKubeletIdentityNotFound)
}

func TestListPodsFromKubeletAPI_StatusErrors(t *testing.T) {
	t.Parallel()

	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()

			pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
			srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(code), code)
			})
			cfg := newTestKubeletAPIConfig(t, pki, srv.URL, "")

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
			require.Error(t, err)
			assert.Nil(t, pods)
			assert.Contains(t, err.Error(), strconv.Itoa(code))
		})
	}
}

func TestListPodsFromKubeletAPI_TLSFailure(t *testing.T) {
	t.Parallel()

	serverPKI := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	// the client trusts a different CA than the one that signed the server cert
	clientPKI := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")

	srv := newTestAPIServer(t, serverPKI, false, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})
	cfg := newTestKubeletAPIConfig(t, clientPKI, srv.URL, "")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
	require.Error(t, err)
	assert.Nil(t, pods)
	assert.Contains(t, err.Error(), "certificate")
}

func TestListPodsFromKubeletAPI_ClientCertRejected(t *testing.T) {
	t.Parallel()

	serverPKI := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	// the client certificate is signed by a CA the server does not trust
	clientPKI := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")

	srv := newTestAPIServer(t, serverPKI, true, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})
	cfg := newTestKubeletAPIConfig(t, clientPKI, srv.URL, "")
	// the client must still verify the server against the server's CA
	cfg.caPool = serverPKI.caPool

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
	require.Error(t, err)
	assert.Nil(t, pods)
}

func TestListPodsFromKubeletAPI_TLSServerName(t *testing.T) {
	t.Parallel()

	// server certificate is valid only for the DNS name, not for 127.0.0.1
	pki := newTestPKI(t, []string{"apiserver.test.local"}, nil, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})

	// without tls-server-name, verification against the dialed IP fails
	cfgNoOverride := newTestKubeletAPIConfig(t, pki, srv.URL, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := listPodsFromKubeletAPI(ctx, cfgNoOverride, 5*time.Second)
	require.Error(t, err, "expected TLS verification to fail without tls-server-name")

	// with tls-server-name, the client dials the configured address but
	// verifies the certificate against the override name
	cfg := newTestKubeletAPIConfig(t, pki, srv.URL, "apiserver.test.local")
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
	require.NoError(t, err)
	assert.Len(t, pods, 2)
}

func TestListPodsFromKubeletAPI_Timeout(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})
	cfg := newTestKubeletAPIConfig(t, pki, srv.URL, "")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 50*time.Millisecond)
	require.Error(t, err)
	assert.Nil(t, pods)
}

func TestListPodsFromKubeletAPI_EmptyPodList(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletEmptyPodsJSON))
	})
	cfg := newTestKubeletAPIConfig(t, pki, srv.URL, "")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
	require.NoError(t, err)
	assert.Empty(t, pods, "a valid empty pod list is a successful response, not an error")
}

func TestListPodsFromKubeletAPI_ConnError(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})
	serverURL := srv.URL
	srv.Close()

	cfg := newTestKubeletAPIConfig(t, pki, serverURL, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsFromKubeletAPI(ctx, cfg, 5*time.Second)
	require.Error(t, err)
	assert.Nil(t, pods)
}

func TestParseKubeletPodList(t *testing.T) {
	t.Parallel()

	pods, err := parseKubeletPodList(strings.NewReader(testKubeletPodsJSON))
	require.NoError(t, err)
	require.Len(t, pods.Items, 2)
	assert.Equal(t, "vector-jldbs", pods.Items[0].Name)
	assert.Equal(t, "test-node", pods.Items[0].Spec.NodeName)

	_, err = parseKubeletPodList(strings.NewReader("not a json"))
	require.Error(t, err)
}
