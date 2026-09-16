package containerd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestRuntimeConfigurationHealth(t *testing.T) {
	const cdi = `{"enableCDI":true,"containerd":{"defaultRuntimeName":"runc","runtimes":{"runc":{}}}}`
	const legacy = `{"enableCDI":false,"containerd":{"defaultRuntimeName":"nvidia","runtimes":{"nvidia":{}}}}`
	const noGPU = `{"enableCDI":false,"containerd":{"defaultRuntimeName":"runc","runtimes":{"runc":{}}}}`
	for _, tt := range []struct {
		name, config, handler, state string
		health                       apiv1.HealthStateType
		reason                       string
	}{
		{"native CDI without toolkit pod", cdi, "", "SANDBOX_READY", apiv1.HealthStateTypeHealthy, "ok"},
		{"native CDI without NVIDIA handler", cdi, "", "SANDBOX_READY", apiv1.HealthStateTypeHealthy, "ok"},
		{"native CDI with explicit runc", cdi, "runc", "SANDBOX_READY", apiv1.HealthStateTypeHealthy, "ok"},
		{"CDI does not hide missing required NVIDIA handler", cdi, "nvidia", "SANDBOX_READY", apiv1.HealthStateTypeUnhealthy, `pod gpu-operator/device-plugin requires runtime handler "nvidia", but containerd does not have it configured`},
		{"other required handler", cdi, "kata", "SANDBOX_READY", apiv1.HealthStateTypeUnhealthy, `pod gpu-operator/device-plugin requires runtime handler "kata", but containerd does not have it configured`},
		{"retired sandbox does not require handler", cdi, "nvidia", "SANDBOX_NOTREADY", apiv1.HealthStateTypeHealthy, "ok"},
		{"legacy NVIDIA default", legacy, "nvidia", "SANDBOX_READY", apiv1.HealthStateTypeHealthy, "ok"},
		{"neither CDI nor NVIDIA default", noGPU, "runc", "SANDBOX_READY", apiv1.HealthStateTypeUnhealthy, "native CDI is disabled and containerd's default runtime is not nvidia"},
		{"missing status config", "", "nvidia", "SANDBOX_READY", apiv1.HealthStateTypeDegraded, "containerd runtime configuration check unavailable"},
		{"incomplete status config", `{"enableCDI":true}`, "nvidia", "SANDBOX_READY", apiv1.HealthStateTypeDegraded, "containerd runtime configuration check unavailable"},
		{"malformed status config", `{`, "nvidia", "SANDBOX_READY", apiv1.HealthStateTypeDegraded, "containerd runtime configuration check unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				ctx:                               t.Context(),
				nvmlInstance:                      &mockNVMLInstance{nvmlExists: true, productName: "GPU"},
				getTimeNowFunc:                    time.Now,
				checkDependencyInstalledFunc:      func() bool { return true },
				getRuntimeConfigFunc:              func() ([]byte, error) { return []byte(tt.config), nil },
				containerToolkitCreationThreshold: 10 * time.Minute,
				listAllSandboxesFunc: func(context.Context, string) ([]PodSandbox, error) {
					pods := []PodSandbox{
						{Namespace: "gpu-operator", Name: "device-plugin", RuntimeHandler: tt.handler, State: tt.state},
						{Name: "nvidia-container-toolkit-daemonset-test", State: "SANDBOX_READY", CreatedAt: time.Now().Add(-time.Hour).UnixNano()},
					}
					if tt.name == "native CDI without toolkit pod" {
						pods = pods[:1]
					}
					return pods, nil
				},
			}
			cr := c.Check().(*checkResult)
			assert.Equal(t, tt.health, cr.health)
			assert.Equal(t, tt.reason, cr.reason)
		})
	}
}

func TestParseRuntimeConfig(t *testing.T) {
	for _, data := range []string{"", "null", "{}", `{"containerd":{"defaultRuntimeName":"runc"}}`, `{"containerd":{"runtimes":{"runc":{}}}}`, `{"enableCDI":"true"}`} {
		_, err := parseRuntimeConfig([]byte(data))
		require.Error(t, err)
	}
}

// The fake server only returns configuration for a verbose request.
type verboseRuntimeServer struct{ fakeRuntimeServer }

func (s *verboseRuntimeServer) Status(_ context.Context, req *runtimeapi.StatusRequest) (*runtimeapi.StatusResponse, error) {
	if !req.Verbose {
		return nil, errors.New("verbose status required")
	}
	return s.statusResp, s.statusErr
}

func TestGetRuntimeConfig(t *testing.T) {
	const config = `{"enableCDI":true,"containerd":{"defaultRuntimeName":"runc","runtimes":{"runc":{}}}}`
	for _, tt := range []struct {
		name string
		info map[string]string
		err  error
	}{
		{"live config", map[string]string{"config": config}, nil},
		{"config unavailable", nil, nil},
		{"status failure", nil, errors.New("unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &verboseRuntimeServer{fakeRuntimeServer{statusResp: &runtimeapi.StatusResponse{Info: tt.info}, statusErr: tt.err}}
			endpoint, cleanup := startFakeRuntimeServer(t, srv)
			defer cleanup()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			got, err := getRuntimeConfig(ctx, endpoint)
			if tt.err != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.info == nil {
				_, err = parseRuntimeConfig(got)
				require.Error(t, err)
				return
			}
			assert.Equal(t, config, string(got))
		})
	}
}

func TestSandboxRuntimeHandler(t *testing.T) {
	pods := convertToPodSandboxes(&runtimeapi.ListPodSandboxResponse{Items: []*runtimeapi.PodSandbox{
		{Id: "pod", Metadata: &runtimeapi.PodSandboxMetadata{Name: "device-plugin", Namespace: "gpu-operator"}, RuntimeHandler: "nvidia", State: runtimeapi.PodSandboxState_SANDBOX_READY},
	}}, &runtimeapi.ListContainersResponse{})
	require.Len(t, pods, 1)
	assert.Equal(t, "nvidia", pods[0].RuntimeHandler)
}
