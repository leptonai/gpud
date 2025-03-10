package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkg_file "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

const DefaultKubeletReadOnlyPort = 10255

// isConnectionRefusedError checks if an error contains "connection refused".
// e.g.,
// Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused
// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
func isConnectionRefusedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "connection refused")
}

func checkKubeletReadOnlyPortHealthz(ctx context.Context, port int) error {
	u := fmt.Sprintf("http://localhost:%d/healthz", port)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checking kubelet read-only port failed %d", resp.StatusCode)
	}

	// make sure it's healthy 'ok' response
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if string(b) != "ok" {
		return fmt.Errorf("kubelet read-only port /healthz expected 'ok', got %q", string(b))
	}

	return nil
}

func checkKubeletInstalled() bool {
	p, err := pkg_file.LocateExecutable("kubelet")
	if err == nil {
		log.Logger.Debugw("kubelet found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("kubelet not found in PATH", "error", err)
	return false
}

// checkKubeletReadOnlyPortListening checks if the kubelet read-only port is listening.
// It first checks if the kubelet is running and then checks if the port is open.
func checkKubeletReadOnlyPortListening(ctx context.Context, port int) bool {
	// check if the TCP port is open/used
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 3*time.Second)
	if err != nil {
		log.Logger.Debugw("tcp port is not open", "port", port, "error", err)
	} else {
		log.Logger.Debugw("tcp port is open", "port", port)
		conn.Close()

		kerr := checkKubeletReadOnlyPortHealthz(ctx, port)
		// check
		if kerr != nil {
			log.Logger.Debugw("kubelet readonly port is not open", "port", port, "error", kerr)
		} else {
			log.Logger.Debugw("auto-detected kubelet readonly port -- configuring k8s pod components", "port", port)

			// "kubelet_pod" requires kubelet read-only port
			// assume if kubelet is running, it opens the most common read-only port 10255
			return true
		}
	}

	return false
}

// returns the node name and the list of pods
func listPodsFromKubeletReadOnlyPort(ctx context.Context, port int) (string, []PodStatus, error) {
	url := fmt.Sprintf("http://localhost:%d/pods", port)
	req, rerr := http.NewRequest(http.MethodGet, url, nil)
	if rerr != nil {
		return "", nil, rerr
	}
	req = req.WithContext(ctx)

	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	pods, err := parsePodsFromKubeletReadOnlyPort(resp.Body)
	if err != nil {
		return "", nil, err
	}
	log.Logger.Debugw("listed pods", "pods", len(pods.Items))

	nodeName := ""
	pss := make([]PodStatus, 0)
	for _, pod := range pods.Items {
		if nodeName == "" {
			nodeName = pod.Spec.NodeName
		}
		pss = append(pss, convertToPodsStatus(pod)...)
	}
	return nodeName, pss, nil
}

func parsePodsFromKubeletReadOnlyPort(r io.Reader) (*corev1.PodList, error) {
	// ref. "pkg/kubelet/server/server.go#encodePods"
	podList := new(corev1.PodList)
	if err := json.NewDecoder(r).Decode(podList); err != nil {
		return nil, err
	}
	return podList, nil
}

func defaultHTTPClient() *http.Client {
	tr := &http.Transport{
		DisableCompression: true,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
}

// Converts the original pod status to the simpler one.
func convertToPodsStatus(pods ...corev1.Pod) []PodStatus {
	statuses := make([]PodStatus, 0, len(pods))
	for _, pod := range pods {
		statuses = append(statuses, convertToPodStatus(pod))
	}
	return statuses
}

func convertToPodStatus(pod corev1.Pod) PodStatus {
	iss := make([]ContainerStatus, 0, len(pod.Status.InitContainerStatuses))
	for _, st := range pod.Status.InitContainerStatuses {
		iss = append(iss, ContainerStatus{
			Name:         st.Name,
			State:        *st.State.DeepCopy(),
			Ready:        st.Ready,
			RestartCount: st.RestartCount,
			Image:        st.Image,
			ContainerID:  st.ContainerID,
		})
	}

	css := make([]ContainerStatus, 0, len(pod.Status.ContainerStatuses))
	for _, st := range pod.Status.ContainerStatuses {
		css = append(css, ContainerStatus{
			Name:         st.Name,
			State:        *st.State.DeepCopy(),
			Ready:        st.Ready,
			RestartCount: st.RestartCount,
			Image:        st.Image,
			ContainerID:  st.ContainerID,
		})
	}

	conds := make([]PodCondition, 0, len(pod.Status.Conditions))
	for _, c := range pod.Status.Conditions {
		conds = append(conds, PodCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}

	return PodStatus{
		ID:                    string(pod.UID),
		Namespace:             pod.Namespace,
		Name:                  pod.Name,
		Phase:                 string(pod.Status.Phase),
		Conditions:            conds,
		Message:               pod.Status.Message,
		Reason:                pod.Status.Reason,
		StartTime:             pod.Status.StartTime,
		InitContainerStatuses: iss,
		ContainerStatuses:     css,
	}
}

// PodStatus represents the simpler pod status from kubelet API.
// ref. https://pkg.go.dev/k8s.io/api/core/v1#PodStatus
type PodStatus struct {
	ID                    string            `json:"id,omitempty"`
	Namespace             string            `json:"namespace,omitempty"`
	Name                  string            `json:"name,omitempty"`
	Phase                 string            `json:"phase,omitempty"`
	Conditions            []PodCondition    `json:"conditions,omitempty"`
	Message               string            `json:"message,omitempty"`
	Reason                string            `json:"reason,omitempty"`
	StartTime             *metav1.Time      `json:"startTime,omitempty"`
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty"`
	ContainerStatuses     []ContainerStatus `json:"containerStatuses,omitempty"`
}

func (s PodStatus) JSON() ([]byte, error) {
	return json.Marshal(s)
}

// ref. https://pkg.go.dev/k8s.io/api/core/v1#PodCondition
type PodCondition struct {
	Type               string      `json:"type,omitempty"`
	Status             string      `json:"status,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
}

// ref. https://pkg.go.dev/k8s.io/api/core/v1#ContainerStatus
// ref. https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status
type ContainerStatus struct {
	Name         string                `json:"name,omitempty"`
	State        corev1.ContainerState `json:"state,omitempty"`
	Ready        bool                  `json:"ready"`
	RestartCount int32                 `json:"restartCount"`
	Image        string                `json:"image,omitempty"`
	ContainerID  string                `json:"containerId,omitempty"`
}
