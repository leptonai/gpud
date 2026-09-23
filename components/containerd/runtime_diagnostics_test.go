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
	"github.com/leptonai/gpud/components"
)

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

func TestRuntimeDiagnosticsPreserveHealthChecks(t *testing.T) {
	const validDisk = `default_runtime_name = "nvidia"
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.nvidia]`
	const invalidDisk = `default_runtime_name = "runc"`
	const cdiOnly = `{"enableCDI":true,"containerd":{"defaultRuntimeName":"runc","runtimes":{"runc":{}}}}`
	const loadedNvidia = `{"enableCDI":true,"containerd":{"defaultRuntimeName":"runc","runtimes":{"runc":{},"nvidia":{}}}}`
	now := time.Now()
	for _, tt := range []struct {
		name, disk, live, handler string
		diskErr, liveErr          error
		age                       time.Duration
		noToolkit                 bool
		expected                  apiv1.HealthStateType
		diagnostic                string
		wantCalls                 int
	}{
		{name: "CDI cannot bypass invalid disk config", disk: invalidDisk, live: cdiOnly, age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: "native CDI enabled=true", wantCalls: 1},
		{name: "missing required handler names pod", disk: invalidDisk, live: cdiOnly, handler: "nvidia", age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: `pod gpu-operator/device-plugin requires runtime handler "nvidia"`, wantCalls: 1},
		{name: "loaded handlers cannot hide broken restart config", disk: invalidDisk, live: loadedNvidia, handler: "nvidia", age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: `configured handlers ["nvidia" "runc"]`, wantCalls: 1},
		{name: "query failure cannot downgrade unhealthy", disk: invalidDisk, liveErr: errors.New("unavailable"), age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: "live containerd runtime diagnostics unavailable", wantCalls: 1},
		{name: "malformed status cannot downgrade unhealthy", disk: invalidDisk, live: "{", age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: "live containerd runtime diagnostics unavailable", wantCalls: 1},
		{name: "missing status cannot downgrade unhealthy", disk: invalidDisk, age: time.Hour, expected: apiv1.HealthStateTypeUnhealthy, diagnostic: "live containerd runtime diagnostics unavailable", wantCalls: 1},
		{name: "healthy config does not query CRI status", disk: validDisk, liveErr: errors.New("must not query"), age: time.Hour, expected: apiv1.HealthStateTypeHealthy},
		{name: "startup grace does not query CRI status", disk: invalidDisk, age: time.Minute, expected: apiv1.HealthStateTypeHealthy},
		{name: "absent toolkit retains original warning", disk: invalidDisk, noToolkit: true, age: time.Hour, expected: apiv1.HealthStateTypeHealthy},
		{name: "disk read failure retains original behavior", diskErr: errors.New("read failed"), age: time.Hour, expected: apiv1.HealthStateTypeHealthy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			c := &component{
				ctx: t.Context(), nvmlInstance: &mockNVMLInstance{nvmlExists: true, productName: "GPU"},
				getTimeNowFunc:                    func() time.Time { return now },
				checkDependencyInstalledFunc:      func() bool { return true },
				containerToolkitCreationThreshold: 10 * time.Minute,
				getContainerdConfigFunc:           func() ([]byte, error) { return []byte(tt.disk), tt.diskErr },
				listAllSandboxesFunc: func(context.Context, string) ([]PodSandbox, error) {
					pods := []PodSandbox{{Namespace: "gpu-operator", Name: "device-plugin", State: "SANDBOX_READY", RuntimeHandler: tt.handler}}
					if !tt.noToolkit {
						pods = append(pods, PodSandbox{Name: "nvidia-container-toolkit-daemonset-test", State: "SANDBOX_READY", CreatedAt: now.Add(-tt.age).UnixNano()})
					}
					return pods, nil
				},
			}
			baseline := c.Check().(*checkResult)
			c.getRuntimeConfigFunc = func() ([]byte, error) { calls++; return []byte(tt.live), tt.liveErr }
			result := c.Check().(*checkResult)
			assert.Equal(t, tt.expected, result.health)
			assert.Equal(t, baseline.health, result.health)
			assert.Equal(t, baseline.err, result.err)
			assert.Equal(t, baseline.suggestedAction, result.suggestedAction)
			assert.Equal(t, tt.wantCalls, calls)
			if tt.diagnostic != "" {
				assert.Contains(t, result.reason, baseline.reason+"; ")
				assert.Contains(t, result.reason, tt.diagnostic)
			} else {
				assert.Equal(t, baseline.reason, result.reason)
			}
		})
	}
}

func TestNewWiresRuntimeDiagnostics(t *testing.T) {
	comp, err := New(&components.GPUdInstance{RootCtx: t.Context()})
	require.NoError(t, err)
	c := comp.(*component)
	defer c.cancel()
	require.NotNil(t, c.getRuntimeConfigFunc)
	require.NotNil(t, c.getContainerdConfigFunc)
}
