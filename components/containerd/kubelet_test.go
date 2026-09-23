package containerd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	cfg, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, clientCertPath, caPath)
	require.NoError(t, err)
	return cfg
}

func TestLoadKubeletAPIConfig(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "https://10.0.0.1:6443", "apiserver.internal")

	cfg, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, clientCertPath, caPath)
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
			_, err := loadKubeletAPIConfig(t.Context(), tt.kubeconfigPath, tt.clientCertPath, tt.caPath)
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

	_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, clientCertPath, caPath)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errKubeletIdentityNotFound,
		"an existing but non-node identity must surface as an error, not as a skip")
	assert.Contains(t, err.Error(), systemNodePrefix)
}

func TestLoadKubeletAPIConfig_NonHTTPS(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	kubeconfigPath, clientCertPath, caPath := writeKubeletCredentials(t, pki, "http://10.0.0.1:8080", "")

	_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, clientCertPath, caPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not https")
}

// writeKubeletKubeconfig writes a kubeconfig with the given cluster and user
// sections, mirroring how different provisioners lay out kubelet's identity.
func writeKubeletKubeconfig(t *testing.T, dir, serverURL, clusterSection, userSection string) string {
	t.Helper()
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
%s
users:
- name: kubelet
  user:
%s
`, clusterSection, userSection)
	path := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(kubeconfig), 0600))
	return path
}

// splitClientCertPEM splits the combined cert+key PEM (the shape of
// kubelet-client-current.pem) into its certificate and key halves.
func splitClientCertPEM(t *testing.T, combined []byte) (certPEM, keyPEM []byte) {
	t.Helper()
	certBlock, rest := pem.Decode(combined)
	require.NotNil(t, certBlock, "combined PEM must start with the certificate")
	keyBlock, _ := pem.Decode(rest)
	require.NotNil(t, keyBlock, "combined PEM must contain the key")
	return pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock)
}

func TestListPodsUsingKubeletIdentity_EmbeddedClientCert(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/pods", r.URL.Path)
		assert.Equal(t, "spec.nodeName=test-node", r.URL.Query().Get("fieldSelector"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})

	// static kubeadm kubelet.conf layout: identity embedded in the kubeconfig,
	// no standalone PEM files at all
	certPEM, keyPEM := splitClientCertPEM(t, pki.clientCertPEM)
	dir := t.TempDir()
	kubeconfigPath := writeKubeletKubeconfig(t, dir, srv.URL,
		fmt.Sprintf("    server: %s\n    certificate-authority-data: %s",
			srv.URL, base64.StdEncoding.EncodeToString(pki.caPEM)),
		fmt.Sprintf("    client-certificate-data: %s\n    client-key-data: %s",
			base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)),
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsUsingKubeletIdentity(ctx, kubeconfigPath, "", "")
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, "vector-jldbs", pods[0].Name)
}

func TestListPodsUsingKubeletIdentity_ExecCredential(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	hostname, err := os.Hostname()
	require.NoError(t, err)
	srv := newTestAPIServer(t, pki, false, func(w http.ResponseWriter, r *http.Request) {
		// token authentication: no client certificate, bearer token required
		assert.Equal(t, "Bearer test-exec-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/nodes/" + hostname:
			_, _ = w.Write([]byte(`{"kind":"Node","apiVersion":"v1"}`))
		case "/api/v1/pods":
			assert.Equal(t, "spec.nodeName="+hostname, r.URL.Query().Get("fieldSelector"))
			_, _ = w.Write([]byte(testKubeletPodsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// fake exec credential plugin, same contract as "aws eks get-token"
	dir := t.TempDir()
	execPath := filepath.Join(dir, "fake-exec")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\nprintf '%s' '{\"kind\":\"ExecCredential\",\"apiVersion\":\"client.authentication.k8s.io/v1beta1\",\"status\":{\"token\":\"test-exec-token\"}}'\n"), 0755))

	// EKS layout: CA as a file reference, identity only via the exec plugin
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caPath, pki.caPEM, 0644))
	kubeconfigPath := writeKubeletKubeconfig(t, dir, srv.URL,
		fmt.Sprintf("    server: %s\n    certificate-authority: %s", srv.URL, caPath),
		fmt.Sprintf("    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      command: %s", execPath),
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsUsingKubeletIdentity(ctx, kubeconfigPath, "", "")
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, "kube-proxy-hfqwt", pods[1].Name)
}

func TestLoadKubeletAPIConfig_NoIdentity(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caPath, pki.caPEM, 0644))
	kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
		"    server: https://10.0.0.1:6443",
		"    {}",
	)

	_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", caPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKubeletIdentityNotFound,
		"a kubeconfig with no client certificate and no exec credential must read as identity-not-found")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestLoadKubeletAPIConfig_FileReferenceClientCert(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	certPEM, keyPEM := splitClientCertPEM(t, pki.clientCertPEM)

	// kubeadm kubelet.conf file-reference layout, all paths relative to the
	// kubeconfig directory
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubelet-client.crt"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubelet-client.key"), keyPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), pki.caPEM, 0644))
	kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
		"    server: https://10.0.0.1:6443\n    certificate-authority: ca.crt",
		"    client-certificate: kubelet-client.crt\n    client-key: kubelet-client.key",
	)

	cfg, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
	require.NoError(t, err)
	require.NotNil(t, cfg.clientCert)
	assert.Equal(t, "test-node", cfg.nodeName)
	assert.Empty(t, cfg.bearerToken)
}

func TestLoadKubeletAPIConfig_Errors(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	pki2 := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:other-node")
	certPEM, keyPEM := splitClientCertPEM(t, pki.clientCertPEM)
	_, key2PEM := splitClientCertPEM(t, pki2.clientCertPEM)

	goodCluster := "    server: https://10.0.0.1:6443\n    certificate-authority-data: " + base64.StdEncoding.EncodeToString(pki.caPEM)
	goodExecUser := func(t *testing.T, dir, body string) string {
		execPath := filepath.Join(dir, "fake-exec")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\n"+body), 0755))
		return "    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      command: " + execPath
	}

	t.Run("unknown cluster in current-context", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster, "    {}")
		// point current-context at a cluster that does not exist
		data, err := os.ReadFile(kubeconfigPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(kubeconfigPath, []byte(strings.Replace(string(data), "cluster: test-cluster", "cluster: nope", 1)), 0600))
		_, err = loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no cluster found")
	})

	t.Run("unknown user in current-context", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster, "    {}")
		data, err := os.ReadFile(kubeconfigPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(kubeconfigPath, []byte(strings.Replace(string(data), "user: kubelet\n", "user: nope\n", 1)), 0600))
		_, err = loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user found")
	})

	t.Run("unparseable server address", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
			"    server: \"https://[::1\"\n    certificate-authority-data: "+base64.StdEncoding.EncodeToString(pki.caPEM), "    {}")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid API server address")
	})

	t.Run("invalid CA data encoding", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
			"    server: https://10.0.0.1:6443\n    certificate-authority-data: \"!!!not-base64!!!\"", "    {}")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate-authority-data")
	})

	t.Run("CA data is not PEM", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
			"    server: https://10.0.0.1:6443\n    certificate-authority-data: "+base64.StdEncoding.EncodeToString([]byte("garbage")), "    {}")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no valid CA certificates")
	})

	t.Run("no CA anywhere", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443",
			"    server: https://10.0.0.1:6443", "    {}")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no cluster CA found")
	})

	t.Run("explicit client cert path holds garbage", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		garbage := filepath.Join(dir, "garbage.pem")
		require.NoError(t, os.WriteFile(garbage, []byte("not a pem"), 0600))
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster, "    {}")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, garbage, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load kubelet client certificate")
	})

	t.Run("embedded cert data not base64", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			"    client-certificate-data: \"!!!\"\n    client-key-data: "+base64.StdEncoding.EncodeToString(keyPEM))
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client-certificate-data")
	})

	t.Run("embedded key data not base64", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			"    client-certificate-data: "+base64.StdEncoding.EncodeToString(certPEM)+"\n    client-key-data: \"!!!\"")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client-key-data")
	})

	t.Run("embedded cert and key mismatch", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			"    client-certificate-data: "+base64.StdEncoding.EncodeToString(certPEM)+"\n    client-key-data: "+base64.StdEncoding.EncodeToString(key2PEM))
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embedded kubelet client certificate")
	})

	t.Run("file-reference cert missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			"    client-certificate: nope.crt\n    client-key: nope.key")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errKubeletIdentityNotFound)
	})

	t.Run("file-reference cert garbage", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.crt"), []byte("x"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.key"), []byte("x"), 0600))
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			"    client-certificate: bad.crt\n    client-key: bad.key")
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load kubelet client certificate")
	})

	t.Run("exec credential fails", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kubeconfigPath := writeKubeletKubeconfig(t, dir, "https://10.0.0.1:6443", goodCluster,
			goodExecUser(t, dir, "exit 1\n"))
		_, err := loadKubeletAPIConfig(t.Context(), kubeconfigPath, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run kubelet exec credential")
	})
}

func TestRunKubeletExecCredential(t *testing.T) {
	t.Parallel()

	writeExec := func(t *testing.T, body string) *kubeletExecSpec {
		t.Helper()
		path := filepath.Join(t.TempDir(), "fake-exec")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755))
		return &kubeletExecSpec{APIVersion: "client.authentication.k8s.io/v1beta1", Command: path}
	}

	t.Run("success with env passthrough", func(t *testing.T) {
		t.Parallel()
		spec := writeExec(t, "printf '%s' \"{\\\"status\\\":{\\\"token\\\":\\\"$MY_TOKEN\\\"}}\"\n")
		spec.Env = []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}{{Name: "MY_TOKEN", Value: "token-from-env"}}
		token, err := runKubeletExecCredential(t.Context(), spec)
		require.NoError(t, err)
		assert.Equal(t, "token-from-env", token)
	})

	t.Run("unsupported apiVersion", func(t *testing.T) {
		t.Parallel()
		spec := writeExec(t, "exit 0\n")
		spec.APIVersion = "v1"
		_, err := runKubeletExecCredential(t.Context(), spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported exec credential apiVersion")
	})

	t.Run("command fails", func(t *testing.T) {
		t.Parallel()
		_, err := runKubeletExecCredential(t.Context(), writeExec(t, "exit 1\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed")
	})

	t.Run("unparseable output", func(t *testing.T) {
		t.Parallel()
		_, err := runKubeletExecCredential(t.Context(), writeExec(t, "echo not-json\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse exec credential")
	})

	t.Run("empty token", func(t *testing.T) {
		t.Parallel()
		_, err := runKubeletExecCredential(t.Context(), writeExec(t, "printf '%s' '{\"status\":{}}'\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty token")
	})
}

func TestListPodsUsingKubeletIdentity_ExecNodeNameRejected(t *testing.T) {
	t.Parallel()

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, false, func(w http.ResponseWriter, r *http.Request) {
		// node object never verifies: the hostname is not a node name here
		w.WriteHeader(http.StatusNotFound)
	})

	dir := t.TempDir()
	execPath := filepath.Join(dir, "fake-exec")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\nprintf '%s' '{\"status\":{\"token\":\"test-exec-token\"}}'\n"), 0755))
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caPath, pki.caPEM, 0644))
	kubeconfigPath := writeKubeletKubeconfig(t, dir, srv.URL,
		fmt.Sprintf("    server: %s\n    certificate-authority: %s", srv.URL, caPath),
		fmt.Sprintf("    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      command: %s", execPath),
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := listPodsUsingKubeletIdentity(ctx, kubeconfigPath, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verifying node name")
}

func TestListPodsFromKubeletAPI_NodeNameResolutionConnError(t *testing.T) {
	t.Parallel()

	// server unreachable: node-name verification cannot even connect
	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, false, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()

	cfg := &kubeletAPIConfig{
		server:      mustParseURL(t, srv.URL),
		caPool:      pki.caPool,
		bearerToken: "test-exec-token",
	}
	_, err := listPodsFromKubeletAPI(t.Context(), cfg, time.Second)
	require.Error(t, err)
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

	t.Run("missing client cert and CA candidates are not errors", func(t *testing.T) {
		t.Parallel()
		// the kubeconfig may embed them or use an exec credential instead
		kubeconfigPath, clientCertPath, caPath, err := resolveKubeletIdentityPaths(
			[]string{existing("kubeconfig")},
			[]string{missing},
			[]string{missing},
		)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "kubeconfig"), kubeconfigPath)
		assert.Empty(t, clientCertPath)
		assert.Empty(t, caPath)
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

func TestListPodsUsingDiscoveredKubeletIdentity_K3sLayout(t *testing.T) {
	// mutates the package-level default candidate paths; do not run in parallel

	pki := newTestPKI(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, "system:node:test-node")
	srv := newTestAPIServer(t, pki, true, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testKubeletPodsJSON))
	})

	// k3s agent layout: kubelet.kubeconfig under the rancher agent dir with
	// absolute file references for the client certificate, key, and server CA
	agentDir := filepath.Join(t.TempDir(), "var", "lib", "rancher", "k3s", "agent")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	certPEM, keyPEM := splitClientCertPEM(t, pki.clientCertPEM)
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "client-kubelet.crt"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "client-kubelet.key"), keyPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "server-ca.crt"), pki.caPEM, 0644))
	kubeconfigPath := writeKubeletKubeconfig(t, agentDir, srv.URL,
		fmt.Sprintf("    server: %s\n    certificate-authority: %s", srv.URL, filepath.Join(agentDir, "server-ca.crt")),
		fmt.Sprintf("    client-certificate: %s\n    client-key: %s", filepath.Join(agentDir, "client-kubelet.crt"), filepath.Join(agentDir, "client-kubelet.key")),
	)
	require.NoError(t, os.Rename(kubeconfigPath, filepath.Join(agentDir, "kubelet.kubeconfig")))

	// no standalone pki discoveries: identity must come from kubeconfig refs
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	origKubeconfig, origClientCert, origCA := defaultKubeletKubeconfigPaths, defaultKubeletClientCertPaths, defaultKubeletCAPaths
	t.Cleanup(func() {
		defaultKubeletKubeconfigPaths = origKubeconfig
		defaultKubeletClientCertPaths = origClientCert
		defaultKubeletCAPaths = origCA
	})
	defaultKubeletKubeconfigPaths = []string{missing, filepath.Join(agentDir, "kubelet.kubeconfig")}
	defaultKubeletClientCertPaths = []string{missing}
	defaultKubeletCAPaths = []string{missing}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pods, err := listPodsUsingDiscoveredKubeletIdentity(ctx)
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, "vector-jldbs", pods[0].Name)
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
