package containerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

	// clientCert is kubelet's current client certificate. Nil when the
	// kubeconfig authenticates with an exec credential instead.
	clientCert *tls.Certificate

	// bearerToken is the token produced by the kubeconfig's exec credential.
	// Empty when client-certificate authentication is used.
	bearerToken string

	// nodeName is this node's name, taken from the client certificate CN
	// ("system:node:<name>"). Empty under exec-credential authentication,
	// where no certificate CN exists; it is then resolved against the API
	// server before the first pod list.
	nodeName string
}

// kubeletKubeconfig is the minimal subset of a clientcmd config needed to
// resolve the API server endpoint and kubelet's client identity exactly as
// kubelet does.
type kubeletKubeconfig struct {
	CurrentContext string `json:"current-context"`
	Contexts       []struct {
		Name    string `json:"name"`
		Context struct {
			Cluster string `json:"cluster"`
			User    string `json:"user"`
		} `json:"context"`
	} `json:"contexts"`
	Clusters []struct {
		Name    string                   `json:"name"`
		Cluster kubeletKubeconfigCluster `json:"cluster"`
	} `json:"clusters"`
	Users []struct {
		Name string                `json:"name"`
		User kubeletKubeconfigUser `json:"user"`
	} `json:"users"`
}

// kubeletKubeconfigCluster is the cluster section of a kubeconfig entry.
type kubeletKubeconfigCluster struct {
	Server                   string `json:"server"`
	TLSServerName            string `json:"tls-server-name,omitempty"`
	CertificateAuthority     string `json:"certificate-authority,omitempty"`
	CertificateAuthorityData string `json:"certificate-authority-data,omitempty"`
}

// kubeletKubeconfigUser is the auth-info section of a kubeconfig user entry.
// Kubelet's identity material varies by provisioner: a rotated client
// certificate under the pki dir (kubeadm), cert/key references or embedded
// data in the kubeconfig (static kubeadm kubelet.conf), or an exec credential
// plugin (EKS, where kubelet authenticates with an IAM-derived token and no
// client certificate exists on the host at all).
type kubeletKubeconfigUser struct {
	ClientCertificate     string           `json:"client-certificate,omitempty"`
	ClientKey             string           `json:"client-key,omitempty"`
	ClientCertificateData string           `json:"client-certificate-data,omitempty"`
	ClientKeyData         string           `json:"client-key-data,omitempty"`
	Exec                  *kubeletExecSpec `json:"exec,omitempty"`
}

// kubeletExecSpec is the exec credential plugin section of a kubeconfig user.
type kubeletExecSpec struct {
	APIVersion string   `json:"apiVersion"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Env        []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"env"`
}

// execCredentialStatus is the subset of client.authentication.k8s.io
// ExecCredential needed for bearer authentication.
type execCredentialStatus struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
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

// resolveKubeletCluster returns the cluster referenced by the kubeconfig's
// current-context, falling back to the sole cluster entry when no
// current-context is set.
func resolveKubeletCluster(cfg kubeletKubeconfig) (*kubeletKubeconfigCluster, error) {
	clusterName := ""
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			clusterName = ctx.Context.Cluster
			break
		}
	}
	for _, c := range cfg.Clusters {
		if c.Name == clusterName || (clusterName == "" && len(cfg.Clusters) == 1) {
			cluster := c.Cluster
			return &cluster, nil
		}
	}
	return nil, fmt.Errorf("no cluster found for current-context %q", cfg.CurrentContext)
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
// first existing path per credential file. Only the kubeconfig is required:
// the client certificate may legitimately live inside the kubeconfig (or be
// replaced by an exec credential), and the CA may be referenced from or
// embedded in the kubeconfig. Returns errKubeletIdentityNotFound when no
// kubeconfig exists at any well-known location, i.e. the node is not part of
// a Kubernetes cluster.
func resolveKubeletIdentityPaths(kubeconfigCandidates, clientCertCandidates, caCandidates []string) (kubeconfigPath, clientCertPath, caPath string, err error) {
	firstExisting := func(candidates []string) string {
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate
			}
		}
		return ""
	}
	kubeconfigPath = firstExisting(kubeconfigCandidates)
	if kubeconfigPath == "" {
		return "", "", "", fmt.Errorf("%w: no kubelet kubeconfig in %v", errKubeletIdentityNotFound, kubeconfigCandidates)
	}
	return kubeconfigPath, firstExisting(clientCertCandidates), firstExisting(caCandidates), nil
}

// resolveKubeletUser returns the auth info of the user referenced by the
// kubeconfig's current-context, falling back to the sole user entry when no
// current-context is set.
func resolveKubeletUser(cfg kubeletKubeconfig) (*kubeletKubeconfigUser, error) {
	userName := ""
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			userName = ctx.Context.User
			break
		}
	}
	for _, u := range cfg.Users {
		if u.Name == userName || (userName == "" && len(cfg.Users) == 1) {
			user := u.User
			return &user, nil
		}
	}
	return nil, fmt.Errorf("no user found for current-context %q", cfg.CurrentContext)
}

// loadKubeletAPIConfig builds the API client configuration from the on-host
// kubelet credentials. clientCertPath and caPath may be empty when discovery
// found no standalone file; the kubeconfig itself is then consulted (embedded
// data, file references, or an exec credential). Returns
// errKubeletIdentityNotFound when no usable kubelet identity exists anywhere.
func loadKubeletAPIConfig(ctx context.Context, kubeconfigPath, clientCertPath, caPath string) (*kubeletAPIConfig, error) {
	kubeconfigBytes, err := readIdentityFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	var kubeconfig kubeletKubeconfig
	if err := yaml.Unmarshal(kubeconfigBytes, &kubeconfig); err != nil {
		return nil, fmt.Errorf("failed to parse kubelet kubeconfig %s: %w", kubeconfigPath, err)
	}
	cluster, err := resolveKubeletCluster(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("invalid kubelet kubeconfig %s: %w", kubeconfigPath, err)
	}
	serverURL, err := url.Parse(cluster.Server)
	if err != nil {
		return nil, fmt.Errorf("invalid API server address %q in %s: %w", cluster.Server, kubeconfigPath, err)
	}
	if serverURL.Scheme != "https" {
		return nil, fmt.Errorf("API server address %q in %s is not https", cluster.Server, kubeconfigPath)
	}

	caBytes, err := resolveCABytes(kubeconfigPath, cluster, caPath)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no valid CA certificates for kubelet kubeconfig %s", kubeconfigPath)
	}

	cfg := &kubeletAPIConfig{
		server:     serverURL,
		serverName: cluster.TLSServerName,
		caPool:     caPool,
	}

	// Client identity, in freshness order: the rotated client certificate
	// under the pki dir when present, then the kubeconfig's own auth info
	// (cert/key data or file references, then an exec credential plugin).
	if clientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientCertPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errKubeletIdentityNotFound, clientCertPath)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load kubelet client certificate %s: %w", clientCertPath, err)
		}
		cfg.clientCert = &cert
	}

	if cfg.clientCert == nil {
		user, err := resolveKubeletUser(kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("invalid kubelet kubeconfig %s: %w", kubeconfigPath, err)
		}
		switch {
		case user.ClientCertificateData != "" && user.ClientKeyData != "":
			certPEM, err := base64.StdEncoding.DecodeString(user.ClientCertificateData)
			if err != nil {
				return nil, fmt.Errorf("failed to decode client-certificate-data in %s: %w", kubeconfigPath, err)
			}
			keyPEM, err := base64.StdEncoding.DecodeString(user.ClientKeyData)
			if err != nil {
				return nil, fmt.Errorf("failed to decode client-key-data in %s: %w", kubeconfigPath, err)
			}
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("failed to load embedded kubelet client certificate in %s: %w", kubeconfigPath, err)
			}
			cfg.clientCert = &cert
		case user.ClientCertificate != "" && user.ClientKey != "":
			dir := filepath.Dir(kubeconfigPath)
			certPath, keyPath := user.ClientCertificate, user.ClientKey
			if !filepath.IsAbs(certPath) {
				certPath = filepath.Join(dir, certPath)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(dir, keyPath)
			}
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: %s", errKubeletIdentityNotFound, certPath)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to load kubelet client certificate %s: %w", certPath, err)
			}
			cfg.clientCert = &cert
		case user.Exec != nil:
			token, err := runKubeletExecCredential(ctx, user.Exec)
			if err != nil {
				return nil, fmt.Errorf("failed to run kubelet exec credential in %s: %w", kubeconfigPath, err)
			}
			cfg.bearerToken = token
		default:
			return nil, fmt.Errorf("%w: %s carries no client certificate or exec credential", errKubeletIdentityNotFound, kubeconfigPath)
		}
	}

	if cfg.clientCert != nil {
		nodeName, err := nodeNameFromClientCert(cfg.clientCert)
		if err != nil {
			return nil, err
		}
		cfg.nodeName = nodeName
	}
	// Under exec-credential authentication no certificate CN exists; nodeName
	// stays empty and is resolved against the API server on first use.

	return cfg, nil
}

// resolveCABytes returns the cluster CA PEM from the discovered CA file when
// present, else from the kubeconfig cluster entry (embedded data or file
// reference, resolved relative to the kubeconfig's directory).
func resolveCABytes(kubeconfigPath string, cluster *kubeletKubeconfigCluster, caPath string) ([]byte, error) {
	if caPath != "" {
		return readIdentityFile(caPath)
	}
	if cluster.CertificateAuthorityData != "" {
		caBytes, err := base64.StdEncoding.DecodeString(cluster.CertificateAuthorityData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode certificate-authority-data in %s: %w", kubeconfigPath, err)
		}
		return caBytes, nil
	}
	if cluster.CertificateAuthority != "" {
		caRef := cluster.CertificateAuthority
		if !filepath.IsAbs(caRef) {
			caRef = filepath.Join(filepath.Dir(kubeconfigPath), caRef)
		}
		return readIdentityFile(caRef)
	}
	return nil, fmt.Errorf("no cluster CA found in or next to kubelet kubeconfig %s", kubeconfigPath)
}

// runKubeletExecCredential executes the kubeconfig's exec credential plugin —
// the same command kubelet itself runs (e.g. "aws eks get-token" on EKS,
// where no client certificate exists on the host) — and returns the bearer
// token from the ExecCredential status. The plugin spec comes from a
// root-owned kubelet kubeconfig at a well-known path; only
// client.authentication.k8s.io exec credentials are honored.
func runKubeletExecCredential(ctx context.Context, spec *kubeletExecSpec) (string, error) {
	if !strings.HasPrefix(spec.APIVersion, "client.authentication.k8s.io/") {
		return "", fmt.Errorf("unsupported exec credential apiVersion %q", spec.APIVersion)
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, spec.Command, spec.Args...)
	for _, env := range spec.Env {
		cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exec credential %q failed: %w", spec.Command, err)
	}
	var cred execCredentialStatus
	if err := json.Unmarshal(out, &cred); err != nil {
		return "", fmt.Errorf("failed to parse exec credential from %q: %w", spec.Command, err)
	}
	if cred.Status.Token == "" {
		return "", fmt.Errorf("exec credential from %q returned an empty token", spec.Command)
	}
	return cred.Status.Token, nil
}

// newKubeletAPIHTTPClient returns an HTTP client that authenticates with
// kubelet's client certificate when present (exec-credential configs carry a
// bearer token instead, applied per request) and verifies the API server
// certificate against the cluster CA.
func newKubeletAPIHTTPClient(cfg *kubeletAPIConfig, timeout time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    cfg.caPool,
	}
	if cfg.clientCert != nil {
		tlsConfig.Certificates = []tls.Certificate{*cfg.clientCert}
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
	cfg, err := loadKubeletAPIConfig(ctx, kubeconfigPath, clientCertPath, caPath)
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
	if cfg.nodeName == "" {
		nodeName, err := resolveNodeName(ctx, cfg, timeout)
		if err != nil {
			return nil, err
		}
		cfg.nodeName = nodeName
	}

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
	if cfg.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.bearerToken)
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

// resolveNodeName determines this node's API server object name when no client
// certificate CN is available (exec-credential authentication). The node name
// matches the OS hostname on EKS-style nodes; the candidate is verified with a
// GET on the node object first — a wrong name must never silently read as
// "zero pods on this node", which would mark every READY sandbox dangling.
func resolveNodeName(ctx context.Context, cfg *kubeletAPIConfig, timeout time.Duration) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to read hostname for kubelet node name resolution: %w", err)
	}
	nodeURL := url.URL{
		Scheme: cfg.server.Scheme,
		Host:   cfg.server.Host,
		Path:   "/api/v1/nodes/" + hostname,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nodeURL.String(), nil)
	if err != nil {
		return "", err
	}
	if cfg.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.bearerToken)
	}
	resp, err := newKubeletAPIHTTPClient(cfg, timeout).Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verifying node name %q against the API server failed with status code %d", hostname, resp.StatusCode)
	}
	return hostname, nil
}
