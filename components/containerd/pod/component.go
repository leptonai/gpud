// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/leptonai/gpud/components"
	containerd_pod_id "github.com/leptonai/gpud/components/containerd/pod/id"
	"github.com/leptonai/gpud/pkg/log"
)

func New(ctx context.Context) components.Component {
	_, cancel := context.WithCancel(ctx)
	c := &component{
		rootCtx:  ctx,
		cancel:   cancel,
		endpoint: defaultContainerRuntimeEndpoint,
	}
	go func() {
		ticker := time.NewTicker(1)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Reset(time.Minute)
			}

			c.checkOnce(time.Now().UTC())
		}
	}()
	return c
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc

	endpoint string

	lastMu      sync.RWMutex
	lastPodTime time.Time
	lastPods    []PodSandbox
	lastErr     string
	lastReason  string
}

func (c *component) Name() string { return containerd_pod_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastPods := c.lastPods
	lastErr := c.lastErr
	lastReason := c.lastReason
	c.lastMu.RUnlock()

	if lastErr != "" {
		return []components.State{
			{
				Name:    containerd_pod_id.Name,
				Health:  components.StateUnhealthy,
				Healthy: false,
				Error:   lastErr,
				Reason:  lastReason,
			},
		}, nil
	}

	o := &Output{
		Pods: lastPods,
	}
	return o.States()
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

	return nil
}

// checkOnce checks the current pods
// run this periodically
func (c *component) checkOnce(ts time.Time) {
	log.Logger.Infow("checking containerd pods", "endpoint", c.endpoint)

	// "rootCtx" here is the root level, create one with shorter timeouts
	// to not block on this checks
	cctx, ccancel := context.WithTimeout(c.rootCtx, 30*time.Second)
	ss, err := listSandboxStatus(cctx, c.endpoint)
	ccancel()
	if err != nil {
		// this is the error from "ListSandboxStatus"
		//
		// e.g.,
		// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
		reason := "failed gRPC call to the containerd socket"
		st, ok := status.FromError(err)
		if ok {
			if st.Code() == codes.Unimplemented {
				reason += "; no CRI configured for containerd"
			}
		}

		c.lastMu.Lock()
		c.lastPodTime = ts
		c.lastPods = nil
		c.lastErr = err.Error()
		c.lastReason = reason
		c.lastMu.Unlock()
		return
	}

	pods := make([]PodSandbox, 0, len(ss))
	for _, s := range ss {
		pods = append(pods, convertToPodSandbox(s))
	}

	c.lastMu.Lock()
	defer c.lastMu.Unlock()
	c.lastPodTime = ts
	c.lastPods = pods
	c.lastErr = ""
	c.lastReason = ""
}

type Output struct {
	Pods []PodSandbox `json:"pods,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func (o *Output) describeReason() string {
	return fmt.Sprintf("total %d pod sandboxes", len(o.Pods))
}

func (o *Output) States() ([]components.State, error) {
	if len(o.Pods) == 0 {
		return []components.State{{
			Name:    containerd_pod_id.Name,
			Health:  components.StateHealthy,
			Healthy: true,
			Reason:  "no output",
		}}, nil
	}

	b, _ := o.JSON()
	return []components.State{{
		Name:    containerd_pod_id.Name,
		Health:  components.StateHealthy,
		Healthy: true,
		Reason:  o.describeReason(),
		ExtraInfo: map[string]string{
			"data":     string(b),
			"encoding": "json",
		},
	}}, nil
}

const (
	defaultSocketFile               = "/run/containerd/containerd.sock"
	defaultContainerRuntimeEndpoint = "unix:///run/containerd/containerd.sock"
)

func listSandboxStatus(ctx context.Context, endpoint string) ([]*runtimeapi.PodSandboxStatusResponse, error) {
	client, imageClient, conn, err := createConn(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	resp, err := client.ListPodSandbox(ctx, &runtimeapi.ListPodSandboxRequest{Filter: &runtimeapi.PodSandboxFilter{}})
	if err != nil {
		return nil, err
	}
	rs := make([]*runtimeapi.PodSandboxStatusResponse, 0, len(resp.Items))
	for _, sandbox := range resp.Items {
		r, err := client.PodSandboxStatus(
			ctx,
			&runtimeapi.PodSandboxStatusRequest{
				PodSandboxId: sandbox.Id,

				// extra info such as process info (not that useful)
				// e.g., "overlayfs\",\"runtimeHandler\":\"\",\"runtimeType\":\"io.containerd.runc.v2\",\"runtimeOptions
				Verbose: false,
			},
		)
		if err != nil {
			// can be safely ignored for current loop if sandbox status fails (e.g., deleted pod)
			log.Logger.Debugw("PodSandboxStatus failed", "error", err)
			continue
		}
		rs = append(rs, r)
		response, err := client.ListContainers(ctx, &runtimeapi.ListContainersRequest{
			Filter: &runtimeapi.ContainerFilter{
				PodSandboxId: sandbox.Id,
			},
		})
		if err != nil {
			return nil, err
		}
		for _, c := range response.Containers {
			image := c.Image
			if imageStatus, err := imageClient.ImageStatus(ctx, &runtimeapi.ImageStatusRequest{
				Image: &runtimeapi.ImageSpec{
					Image:       c.ImageRef,
					Annotations: nil,
				},
				Verbose: false,
			}); err == nil && imageStatus.Image != nil {
				if len(imageStatus.Image.RepoTags) > 0 {
					image.UserSpecifiedImage = strings.Join(imageStatus.Image.RepoTags, ",")
				} else {
					image.UserSpecifiedImage = strings.Join(imageStatus.Image.RepoDigests, ",")
				}
			}
			r.ContainersStatuses = append(r.ContainersStatuses, &runtimeapi.ContainerStatus{
				Id:          c.Id,
				Metadata:    c.Metadata,
				State:       c.State,
				CreatedAt:   c.CreatedAt,
				Image:       c.Image,
				ImageRef:    c.ImageRef,
				Labels:      c.Labels,
				Annotations: c.Annotations,
				ImageId:     c.ImageId,
			})

		}
	}
	return rs, nil
}

// the original "PodSandboxStatusResponse" has a lot of fields, we only need a few of them
func convertToPodSandbox(resp *runtimeapi.PodSandboxStatusResponse) PodSandbox {
	status := resp.GetStatus()
	pod := PodSandbox{
		ID:        status.Id,
		Name:      status.Metadata.Name,
		Namespace: status.Metadata.Namespace,
		State:     status.State.String(),
		Info:      resp.GetInfo(),
	}
	for _, c := range resp.ContainersStatuses {
		pod.Containers = append(pod.Containers, convertContainerStatus(c))
	}
	return pod
}

func convertContainerStatus(c *runtimeapi.ContainerStatus) PodSandboxContainerStatus {
	ret := PodSandboxContainerStatus{
		ID:        c.Id,
		Name:      c.Metadata.Name,
		CreatedAt: c.CreatedAt,
		State:     c.State.String(),
		LogPath:   c.LogPath,
		ExitCode:  c.ExitCode,
		Reason:    c.Reason,
		Message:   c.Message,
	}
	if c.Image != nil {
		ret.Image = c.Image.UserSpecifiedImage
	}
	return ret
}

// PodSandbox represents the pod information fetched from the local container runtime.
// Simplified version of k8s.io/cri-api/pkg/apis/runtime/v1.PodSandbox.
// ref. https://pkg.go.dev/k8s.io/cri-api/pkg/apis/runtime/v1#ListPodSandboxResponse
type PodSandbox struct {
	ID         string                      `json:"id,omitempty"`
	Namespace  string                      `json:"namespace,omitempty"`
	Name       string                      `json:"name,omitempty"`
	State      string                      `json:"state,omitempty"`
	Info       map[string]string           `json:"info,omitempty"`
	Containers []PodSandboxContainerStatus `json:"containers,omitempty"`
}

func (s PodSandbox) JSON() ([]byte, error) {
	return json.Marshal(s)
}

// ref. https://pkg.go.dev/k8s.io/cri-api/pkg/apis/runtime/v1#ContainerStatus
type PodSandboxContainerStatus struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Image     string `json:"image,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	State     string `json:"state,omitempty"`
	LogPath   string `json:"logPath,omitempty"`
	ExitCode  int32  `json:"exitCode,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
}
