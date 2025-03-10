// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	kubelet_pod_id "github.com/leptonai/gpud/components/kubelet/pod/id"
	pkg_file "github.com/leptonai/gpud/pkg/file"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	kubeletReadOnlyPort int

	// In case the kubelet does not open the read-only port, we ignore such errors as
	// 'Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused'.
	ignoreConnectionErrors bool

	lastMu   sync.RWMutex
	lastData Data
}

func New(ctx context.Context, kubeletReadOnlyPort int, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                    cctx,
		cancel:                 cancel,
		kubeletReadOnlyPort:    kubeletReadOnlyPort,
		ignoreConnectionErrors: ignoreConnectionErrors,
	}
	return c
}

var _ components.Component = &component{}

func (c *component) Name() string { return kubelet_pod_id.Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates(c.ignoreConnectionErrors)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking kubelet pods")
	d := Data{
		ts: time.Now().UTC(),
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	d.KubeletPidFound = process.CheckRunningByPid(cctx, "kubelet")
	ccancel()

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.NodeName, d.Pods, d.err = listPodsFromKubeletReadOnlyPort(cctx, c.kubeletReadOnlyPort)
	ccancel()

	d.connErr = isConnectionRefusedError(d.err)

	if d.err != nil {
		components_metrics.SetGetFailed(kubelet_pod_id.Name)
	} else {
		components_metrics.SetGetSuccess(kubelet_pod_id.Name)
	}

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

type Data struct {
	// KubeletPidFound is true if the kubelet pid is found.
	KubeletPidFound bool `json:"kubelet_pid_found"`
	// NodeName is the name of the node.
	NodeName string `json:"node_name,omitempty"`
	// Pods is the list of pods on the node.
	Pods []PodStatus `json:"pods,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
	// set to true if the error is the connection error to kubelet
	connErr bool `json:"-"`
}

func (d *Data) Reason() string {
	if d.err != nil {
		if d.connErr {
			// e.g.,
			// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
			return fmt.Sprintf("connection error to node %q -- %v", d.NodeName, d.err)
		}

		return fmt.Sprintf("failed to list pods from kubelet read-only port -- %v", d.err)
	}

	return fmt.Sprintf("total %d pods (node %s)", len(d.Pods), d.NodeName)
}

func (d *Data) getHealth(ignoreConnErr bool) (string, bool) {
	healthy := d.err == nil
	if d.err != nil && d.connErr && ignoreConnErr {
		healthy = true
	}
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates(ignoreConnErr bool) ([]components.State, error) {
	state := components.State{
		Name:   kubelet_pod_id.Name,
		Reason: d.Reason(),
	}
	state.Health, state.Healthy = d.getHealth(ignoreConnErr)

	if len(d.Pods) == 0 { // no pod found yet
		return []components.State{state}, nil
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}

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

const DefaultKubeletReadOnlyPort = 10255

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

// CheckKubeletReadOnlyPortListening checks if the kubelet read-only port is listening.
// It first checks if the kubelet is running and then checks if the port is open.
func CheckKubeletReadOnlyPortListening(ctx context.Context, port int) bool {
	if runtime.GOOS != "linux" {
		log.Logger.Debugw("ignoring default kubelet checking since it's not linux", "os", runtime.GOOS)
		return false
	}

	p, err := pkg_file.LocateExecutable("kubelet")
	if err == nil {
		log.Logger.Debugw("kubelet found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("kubelet not found in PATH -- fallback to kubelet run checks", "error", err)

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
