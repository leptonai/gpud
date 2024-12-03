package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Output struct {
	NodeName string      `json:"node_name,omitempty"`
	Pods     []PodStatus `json:"pods,omitempty"`

	KubeletPidFound bool   `json:"kubelet_pid_found"`
	ConnectionError string `json:"connection_error,omitempty"`
	Message         string `json:"message,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNamePod = "pod"

	StateKeyPodData           = "data"
	StateKeyPodEncoding       = "encoding"
	StateValuePodEncodingJSON = "json"
)

func ParseStatePod(m map[string]string) (*Output, error) {
	o := &Output{}
	data := m[StateKeyPodData]
	if err := json.Unmarshal([]byte(data), o); err != nil {
		return nil, err
	}
	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNamePod:
			return ParseStatePod(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no pod state found")
}

func (o *Output) describeReason() string {
	if o.ConnectionError != "" {
		// e.g.,
		// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
		return fmt.Sprintf("connection error to node %q -- %s", o.NodeName, o.ConnectionError)
	}
	return fmt.Sprintf("total %d pods (node %s)", len(o.Pods), o.NodeName)
}

func (o *Output) States(cfg Config) ([]components.State, error) {
	healthy := o.ConnectionError == ""
	if cfg.IgnoreConnectionErrors {
		healthy = true
	}

	b, _ := o.JSON()

	return []components.State{{
		Name:    StateNamePod,
		Healthy: healthy,
		Reason:  o.describeReason(),
		ExtraInfo: map[string]string{
			StateKeyPodData:     string(b),
			StateKeyPodEncoding: StateValuePodEncodingJSON,
		},
	}}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller

	defaultPollerCloseOnce sync.Once
	defaultPollerc         = make(chan any)
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet(cfg))
	})
}

func GetDefaultPoller() query.Poller {
	return defaultPoller
}

func DefaultPollerReady() <-chan any {
	return defaultPollerc
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		// check if a process named "kubelet" is running
		kubeletRunning := false
		if err := exec.Command("pidof", "kubelet").Run(); err == nil {
			kubeletRunning = true
		} else {
			log.Logger.Warnw("kubelet process not found, assuming kubelet is not running", "error", err)
		}

		// "ctx" here is the root level, create one with shorter timeouts
		// to not block on this checks
		cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
		pods, err := ListFromKubeletReadOnlyPort(cctx, cfg.Port)
		ccancel()
		if err != nil {
			o := &Output{
				KubeletPidFound: kubeletRunning,
				Message:         "failed to list pods from kubelet read-only port (maybe readOnlyPort not set in kubelet config file) -- " + err.Error(),
			}

			// e.g.,
			// Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused
			// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
			if strings.Contains(err.Error(), "connection refused") {
				o.ConnectionError = err.Error()
			}

			return o, nil
		}
		log.Logger.Debugw("listed pods", "pods", len(pods.Items))

		nodeName := ""
		pss := make([]PodStatus, 0)
		for _, pod := range pods.Items {
			if nodeName == "" {
				nodeName = pod.Spec.NodeName
			}
			pss = append(pss, ConvertToPodsStatus(pod)...)
		}

		return &Output{
			NodeName:        nodeName,
			Pods:            pss,
			KubeletPidFound: kubeletRunning,
		}, nil
	}
}

const DefaultKubeletReadOnlyPort = 10255

func CheckKubeletReadOnlyPort(ctx context.Context, port int) error {
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

func ListFromKubeletReadOnlyPort(ctx context.Context, port int) (*corev1.PodList, error) {
	url := fmt.Sprintf("http://localhost:%d/pods", port)
	req, rerr := http.NewRequest(http.MethodGet, url, nil)
	if rerr != nil {
		return nil, rerr
	}
	req = req.WithContext(ctx)

	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parsePodsFromKubeletReadOnlyPort(resp.Body)
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
func ConvertToPodsStatus(pods ...corev1.Pod) []PodStatus {
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

func ParsePodStatusJSON(b []byte) (*PodStatus, error) {
	pod := new(PodStatus)
	if err := json.Unmarshal(b, pod); err != nil {
		return nil, err
	}
	return pod, nil
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
