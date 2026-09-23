package containerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/log"
)

// Well-known kubelet credential locations, in probe order. No single layout
// is universal: Lepton/kubeadm-style images keep a root kubeconfig and the
// identity PEM under /etc/kubernetes, while EKS and most distro provisioners
// keep kubelet's kubeconfig and rotated client certificate under its data
// directory /var/lib/kubelet. Probing each file independently tolerates mixed
// layouts; a host with none of them is not a Kubernetes node.
var (
	// defaultKubeletKubeconfigPaths are the kubeconfigs kubelet itself uses
	// to reach the cluster API server.
	defaultKubeletKubeconfigPaths = []string{
		"/root/.kube/config",
		"/etc/kubernetes/kubelet.conf",
		"/var/lib/kubelet/kubeconfig",
	}

	// defaultKubeletClientCertPaths hold kubelet's current client certificate
	// and private key as a combined PEM. Kubelet's client-certificate rotation
	// keeps this file current, so it is re-read on every check.
	defaultKubeletClientCertPaths = []string{
		"/etc/kubernetes/pki/kubelet-client-current.pem",
		"/var/lib/kubelet/pki/kubelet-client-current.pem",
	}

	// defaultKubeletCAPaths are the cluster CA certificate that kubelet
	// trusts. Both EKS and kubeadm write it under /etc/kubernetes/pki.
	defaultKubeletCAPaths = []string{
		"/etc/kubernetes/pki/ca.crt",
	}
)

const (
	// kubeletAPITimeout bounds each pod-list request to the API server.
	kubeletAPITimeout = 30 * time.Second

	// systemNodePrefix is the CN prefix of kubelet client certificates.
	systemNodePrefix = "system:node:"
)

// errKubeletIdentityNotFound indicates that the host carries no kubelet client
// credentials, i.e. the node is not part of a Kubernetes cluster and dangling
// pod detection does not apply. Callers must treat this differently from a
// failed query against an existing identity.
var errKubeletIdentityNotFound = errors.New("kubelet client identity not found")

// kubeletPodStatus represents the minimal pod status needed for dangling pod
// detection.
type kubeletPodStatus struct {
	Namespace string
	Name      string
}

// kubeletAPIConfig describes how to list this node's pods from the API server
// using kubelet's own client identity.
type kubeletAPIConfig struct {
	// server is the API server endpoint from the kubelet kubeconfig.
	server *url.URL

	// serverName optionally overrides TLS verification (kubeconfig
	// "tls-server-name"); the connection is still dialed to server.
	serverName string

	// caPool verifies the API server certificate. Never nil.
	caPool *x509.CertPool

	// clientCert is kubelet's current client certificate.
	clientCert tls.Certificate

	// nodeName is this node's name, taken from the client certificate CN
	// ("system:node:<name>").
	nodeName string
}

// kubeletKubeconfig is the minimal subset of a clientcmd config needed to
// resolve the API server endpoint exactly as kubelet does.
type kubeletKubeconfig struct {
	CurrentContext string `json:"current-context"`
	Contexts       []struct {
		Name    string `json:"name"`
		Context struct {
			Cluster string `json:"cluster"`
		} `json:"context"`
	} `json:"contexts"`
	Clusters []struct {
		Name    string `json:"name"`
		Cluster struct {
			Server        string `json:"server"`
			TLSServerName string `json:"tls-server-name,omitempty"`
		} `json:"cluster"`
	} `json:"clusters"`
}

// readIdentityFile reads a kubelet credential file, mapping a missing file to
// errKubeletIdentityNotFound.
func readIdentityFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", errKubeletIdentityNotFound, path)
	}
	return b, err
}

// resolveKubeletCluster returns the server endpoint of the cluster referenced
// by the kubeconfig's current-context, falling back to the sole cluster entry
// when no current-context is set.
func resolveKubeletCluster(cfg kubeletKubeconfig) (server string, tlsServerName string, err error) {
	clusterName := ""
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			clusterName = ctx.Context.Cluster
			break
		}
	}
	for _, c := range cfg.Clusters {
		if c.Name == clusterName || (clusterName == "" && len(cfg.Clusters) == 1) {
			return c.Cluster.Server, c.Cluster.TLSServerName, nil
		}
	}
	return "", "", fmt.Errorf("no cluster found for current-context %q", cfg.CurrentContext)
}

// nodeNameFromClientCert extracts the node name from the kubelet client
// certificate CN ("system:node:<name>").
func nodeNameFromClientCert(cert *tls.Certificate) (string, error) {
	if len(cert.Certificate) == 0 {
		return "", errors.New("kubelet client certificate has no certificate blocks")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return "", fmt.Errorf("failed to parse kubelet client certificate: %w", err)
	}
	cn := leaf.Subject.CommonName
	if !strings.HasPrefix(cn, systemNodePrefix) {
		return "", fmt.Errorf("kubelet client certificate CN %q missing %q prefix", cn, systemNodePrefix)
	}
	return strings.TrimPrefix(cn, systemNodePrefix), nil
}

// resolveKubeletIdentityPaths probes each candidate list and returns the
// first existing path per credential file. Returns errKubeletIdentityNotFound
// when any credential file exists at none of its well-known locations, i.e.
// the node is not part of a Kubernetes cluster.
func resolveKubeletIdentityPaths(kubeconfigCandidates, clientCertCandidates, caCandidates []string) (kubeconfigPath, clientCertPath, caPath string, err error) {
	firstExisting := func(candidates []string) (string, error) {
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("%w: none of %v exists", errKubeletIdentityNotFound, candidates)
	}
	if kubeconfigPath, err = firstExisting(kubeconfigCandidates); err != nil {
		return "", "", "", err
	}
	if clientCertPath, err = firstExisting(clientCertCandidates); err != nil {
		return "", "", "", err
	}
	if caPath, err = firstExisting(caCandidates); err != nil {
		return "", "", "", err
	}
	return kubeconfigPath, clientCertPath, caPath, nil
}

// loadKubeletAPIConfig builds the API client configuration from the on-host
// kubelet credentials. Returns errKubeletIdentityNotFound when any of the
// credential files does not exist.
func loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath string) (*kubeletAPIConfig, error) {
	kubeconfigBytes, err := readIdentityFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	var kubeconfig kubeletKubeconfig
	if err := yaml.Unmarshal(kubeconfigBytes, &kubeconfig); err != nil {
		return nil, fmt.Errorf("failed to parse kubelet kubeconfig %s: %w", kubeconfigPath, err)
	}
	server, tlsServerName, err := resolveKubeletCluster(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("invalid kubelet kubeconfig %s: %w", kubeconfigPath, err)
	}
	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("invalid API server address %q in %s: %w", server, kubeconfigPath, err)
	}
	if serverURL.Scheme != "https" {
		return nil, fmt.Errorf("API server address %q in %s is not https", server, kubeconfigPath)
	}

	caBytes, err := readIdentityFile(caPath)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no valid CA certificates in %s", caPath)
	}

	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientCertPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", errKubeletIdentityNotFound, clientCertPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load kubelet client certificate %s: %w", clientCertPath, err)
	}

	nodeName, err := nodeNameFromClientCert(&clientCert)
	if err != nil {
		return nil, err
	}

	return &kubeletAPIConfig{
		server:     serverURL,
		serverName: tlsServerName,
		caPool:     caPool,
		clientCert: clientCert,
		nodeName:   nodeName,
	}, nil
}

// newKubeletAPIHTTPClient returns an HTTP client that authenticates with
// kubelet's client certificate and verifies the API server certificate against
// the cluster CA.
func newKubeletAPIHTTPClient(cfg *kubeletAPIConfig, timeout time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      cfg.caPool,
		Certificates: []tls.Certificate{cfg.clientCert},
	}
	transport := &http.Transport{
		Proxy:              http.ProxyFromEnvironment,
		DisableCompression: true,
		TLSClientConfig:    tlsConfig,
	}
	if cfg.serverName != "" {
		// Equivalent to curl --resolve <name>:<port>:<host>: dial the
		// configured server address but verify the certificate against
		// tls-server-name.
		tlsConfig.ServerName = cfg.serverName
		dialAddr := cfg.server.Host
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", dialAddr)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// listPodsUsingDiscoveredKubeletIdentity resolves kubelet's on-host client
// credentials from their well-known locations and lists this node's pods from
// the API server. Discovery runs on every call: credential files appear at
// different times during node provisioning and kubelet rotates its client
// certificate, so no probe result is cached.
func listPodsUsingDiscoveredKubeletIdentity(ctx context.Context) ([]kubeletPodStatus, error) {
	kubeconfigPath, clientCertPath, caPath, err := resolveKubeletIdentityPaths(defaultKubeletKubeconfigPaths, defaultKubeletClientCertPaths, defaultKubeletCAPaths)
	if err != nil {
		return nil, err
	}
	log.Logger.Debugw("resolved kubelet client identity", "kubeconfig", kubeconfigPath, "clientCert", clientCertPath, "ca", caPath)
	return listPodsUsingKubeletIdentity(ctx, kubeconfigPath, clientCertPath, caPath)
}

// listPodsUsingKubeletIdentity loads kubelet's on-host client credentials and
// lists this node's pods from the API server. Credentials are re-read on every
// call because kubelet rotates its client certificate.
func listPodsUsingKubeletIdentity(ctx context.Context, kubeconfigPath, clientCertPath, caPath string) ([]kubeletPodStatus, error) {
	cfg, err := loadKubeletAPIConfig(kubeconfigPath, clientCertPath, caPath)
	if err != nil {
		return nil, err
	}
	return listPodsFromKubeletAPI(ctx, cfg, kubeletAPITimeout)
}

// listPodsFromKubeletAPI lists this node's pods from the API server using
// kubelet's client identity. The API server view substitutes for kubelet's
// local pod view: graceful deletions keep the pod object until kubelet
// confirms teardown, and the sustained-absence grace period in
// danglingPodCount absorbs the force-delete divergence window.
func listPodsFromKubeletAPI(ctx context.Context, cfg *kubeletAPIConfig, timeout time.Duration) ([]kubeletPodStatus, error) {
	podsURL := url.URL{
		Scheme:   cfg.server.Scheme,
		Host:     cfg.server.Host,
		Path:     "/api/v1/pods",
		RawQuery: url.Values{"fieldSelector": []string{"spec.nodeName=" + cfg.nodeName}}.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podsURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := newKubeletAPIHTTPClient(cfg, timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing pods for node %q failed with status code %d", cfg.nodeName, resp.StatusCode)
	}

	pods, err := parseKubeletPodList(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Logger.Infow("listed pods from API server using kubelet client identity", "node", cfg.nodeName, "pods", len(pods.Items))

	pss := make([]kubeletPodStatus, 0, len(pods.Items))
	for _, pod := range pods.Items {
		pss = append(pss, kubeletPodStatus{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		})
	}
	return pss, nil
}

func parseKubeletPodList(r io.Reader) (*corev1.PodList, error) {
	podList := new(corev1.PodList)
	if err := json.NewDecoder(r).Decode(podList); err != nil {
		return nil, err
	}
	return podList, nil
}
