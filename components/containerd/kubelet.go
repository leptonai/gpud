package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/leptonai/gpud/pkg/log"
)

// defaultKubeletReadOnlyPort is the kubelet read-only port used for pod listing.
const defaultKubeletReadOnlyPort = 10255

// kubeletPodStatus represents the minimal pod status from the kubelet read-only API.
// Only the fields needed for dangling pod detection are kept.
type kubeletPodStatus struct {
	Namespace string
	Name      string
}

// listPodsFromKubeletReadOnlyPort returns the list of pods from the kubelet read-only port.
func listPodsFromKubeletReadOnlyPort(ctx context.Context, port int) ([]kubeletPodStatus, error) {
	url := fmt.Sprintf("http://localhost:%d/pods", port)
	req, rerr := http.NewRequest(http.MethodGet, url, nil)
	if rerr != nil {
		return nil, rerr
	}
	req = req.WithContext(ctx)

	resp, err := defaultKubeletHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	pods, err := parsePodsFromKubeletReadOnlyPort(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Logger.Debugw("listed pods from kubelet read-only port", "pods", len(pods.Items))

	pss := make([]kubeletPodStatus, 0, len(pods.Items))
	for _, pod := range pods.Items {
		pss = append(pss, kubeletPodStatus{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		})
	}
	return pss, nil
}

func parsePodsFromKubeletReadOnlyPort(r io.Reader) (*corev1.PodList, error) {
	// ref. "pkg/kubelet/server/server.go#encodePods"
	podList := new(corev1.PodList)
	if err := json.NewDecoder(r).Decode(podList); err != nil {
		return nil, err
	}
	return podList, nil
}

func defaultKubeletHTTPClient() *http.Client {
	tr := &http.Transport{
		Proxy:              http.ProxyFromEnvironment,
		DisableCompression: true,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
}
