package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	containerd_pod_id "github.com/leptonai/gpud/components/containerd/pod/id"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type Output struct {
	Pods []PodSandbox `json:"pods,omitempty"`
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
	StateNameContainerdPod = "containerd_pod"

	StateKeyContainerdPodID        = "id"
	StateKeyContainerdPodName      = "name"
	StateKeyContainerdPodNamespace = "namespace"
	StateKeyContainerdPodState     = "state"

	StateKeyContainerdPodData           = "data"
	StateKeyContainerdPodEncoding       = "encoding"
	StateValueContainerdPodEncodingJSON = "json"
)

func ParseStatePodSandbox(m map[string]string) (PodSandbox, error) {
	pod := PodSandbox{}
	pod.ID = m[StateKeyContainerdPodID]
	pod.Name = m[StateKeyContainerdPodName]
	pod.Namespace = m[StateKeyContainerdPodNamespace]
	pod.State = m[StateKeyContainerdPodState]

	data := m[StateKeyContainerdPodData]
	if err := json.Unmarshal([]byte(data), &pod); err != nil {
		return PodSandbox{}, err
	}
	return pod, nil
}

func (o *Output) describeReason() string {
	return fmt.Sprintf("total %d pod sandboxes", len(o.Pods))
}

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	return []components.State{{
		Name:    StateNameContainerdPod,
		Healthy: true,
		Reason:  o.describeReason(),
		ExtraInfo: map[string]string{
			StateKeyContainerdPodData:     string(b),
			StateKeyContainerdPodEncoding: StateValueContainerdPodEncodingJSON,
		},
	}}, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateNameContainerdPod:
			pod, err := ParseStatePodSandbox(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Pods = append(o.Pods, pod)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(containerd_pod_id.Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(containerd_pod_id.Name)
			} else {
				components_metrics.SetGetSuccess(containerd_pod_id.Name)
			}
		}()

		// "ctx" here is the root level, create one with shorter timeouts
		// to not block on this checks
		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		ss, err := ListSandboxStatus(cctx, cfg.Endpoint)
		ccancel()
		if err != nil {
			return nil, err
		}

		pods := make([]PodSandbox, 0)
		for _, s := range ss {
			pods = append(pods, ConvertToPodSandbox(s))
		}
		return &Output{Pods: pods}, nil
	}
}

const (
	DefaultSocketFile               = "/run/containerd/containerd.sock"
	DefaultContainerRuntimeEndpoint = "unix:///run/containerd/containerd.sock"
)

func ListSandboxStatus(ctx context.Context, endpoint string) ([]*runtimeapi.PodSandboxStatusResponse, error) {
	client, imageClient, conn, err := Connect(ctx, endpoint)
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
			return nil, err
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
func ConvertToPodSandbox(resp *runtimeapi.PodSandboxStatusResponse) PodSandbox {
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
