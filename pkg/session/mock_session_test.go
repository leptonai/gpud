package session

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfinibanditypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	componentsnvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentstemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/pkg/update"
)

// mockNvmlInstanceForMockey implements nvidianvml.Instance for mockey tests.
type mockNvmlInstanceForMockey struct{}

func (m *mockNvmlInstanceForMockey) NVMLExists() bool                  { return true }
func (m *mockNvmlInstanceForMockey) Library() nvmllib.Library          { return nil }
func (m *mockNvmlInstanceForMockey) Devices() map[string]device.Device { return nil }
func (m *mockNvmlInstanceForMockey) ProductName() string               { return "mock-gpu" }
func (m *mockNvmlInstanceForMockey) Architecture() string              { return "mock-arch" }
func (m *mockNvmlInstanceForMockey) Brand() string                     { return "mock-brand" }
func (m *mockNvmlInstanceForMockey) DriverVersion() string             { return "mock-version" }
func (m *mockNvmlInstanceForMockey) DriverMajor() int                  { return 1 }
func (m *mockNvmlInstanceForMockey) CUDAVersion() string               { return "mock-cuda" }
func (m *mockNvmlInstanceForMockey) FabricManagerSupported() bool      { return false }
func (m *mockNvmlInstanceForMockey) FabricStateSupported() bool        { return false }
func (m *mockNvmlInstanceForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNvmlInstanceForMockey) Shutdown() error  { return nil }
func (m *mockNvmlInstanceForMockey) InitError() error { return nil }

// --- OpOption tests ---

func TestMockeyOpOption_WithMachineID(t *testing.T) {
	mockey.PatchConvey("WithMachineID sets machine ID on Op", t, func() {
		op := &Op{}
		opt := WithMachineID("test-machine-123")
		opt(op)
		assert.Equal(t, "test-machine-123", op.machineID)
	})
}

func TestMockeyOpOption_WithPipeInterval(t *testing.T) {
	mockey.PatchConvey("WithPipeInterval sets pipe interval on Op", t, func() {
		op := &Op{}
		opt := WithPipeInterval(5 * time.Second)
		opt(op)
		assert.Equal(t, 5*time.Second, op.pipeInterval)
	})
}

func TestMockeyOpOption_WithEnableAutoUpdate(t *testing.T) {
	mockey.PatchConvey("WithEnableAutoUpdate sets auto update flag on Op", t, func() {
		op := &Op{}
		opt := WithEnableAutoUpdate(true)
		opt(op)
		assert.True(t, op.enableAutoUpdate)
	})
}

func TestMockeyOpOption_WithSkipUpdateConfig(t *testing.T) {
	mockey.PatchConvey("WithSkipUpdateConfig sets skip update config flag on Op", t, func() {
		op := &Op{}
		opt := WithSkipUpdateConfig(true)
		opt(op)
		assert.True(t, op.skipUpdateConfig)
	})
}

func TestMockeyOpOption_WithAutoUpdateExitCode(t *testing.T) {
	mockey.PatchConvey("WithAutoUpdateExitCode sets exit code on Op", t, func() {
		op := &Op{}
		opt := WithAutoUpdateExitCode(42)
		opt(op)
		assert.Equal(t, 42, op.autoUpdateExitCode)
	})
}

func TestMockeyOpOption_WithDataDir(t *testing.T) {
	mockey.PatchConvey("WithDataDir sets data directory on Op", t, func() {
		op := &Op{}
		opt := WithDataDir("/tmp/test-data")
		opt(op)
		assert.Equal(t, "/tmp/test-data", op.dataDir)
	})
}

func TestMockeyOpOption_WithDBInMemory(t *testing.T) {
	mockey.PatchConvey("WithDBInMemory sets in-memory flag on Op", t, func() {
		op := &Op{}
		opt := WithDBInMemory(true)
		opt(op)
		assert.True(t, op.dbInMemory)
	})
}

func TestMockeyOpOption_WithNvidiaInstance(t *testing.T) {
	mockey.PatchConvey("WithNvidiaInstance sets NVML instance on Op", t, func() {
		op := &Op{}
		inst := &mockNvmlInstanceForMockey{}
		opt := WithNvidiaInstance(inst)
		opt(op)
		assert.Equal(t, inst, op.nvmlInstance)
	})
}

func TestMockeyOpOption_WithAuditLogger(t *testing.T) {
	mockey.PatchConvey("WithAuditLogger sets audit logger on Op", t, func() {
		op := &Op{}
		logger := log.NewNopAuditLogger()
		opt := WithAuditLogger(logger)
		opt(op)
		assert.Equal(t, logger, op.auditLogger)
	})
}

func TestMockeyOpOption_ApplyOptsDefaults(t *testing.T) {
	mockey.PatchConvey("applyOpts sets defaults correctly", t, func() {
		op := &Op{}
		err := op.applyOpts(nil)
		require.NoError(t, err)
		assert.Equal(t, -1, op.autoUpdateExitCode)
		assert.NotNil(t, op.auditLogger)
	})
}

func TestMockeyOpOption_ApplyOptsAutoUpdateError(t *testing.T) {
	mockey.PatchConvey("applyOpts returns error when auto update disabled but exit code set", t, func() {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithEnableAutoUpdate(false),
			WithAutoUpdateExitCode(1),
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAutoUpdateDisabledButExitCodeSet)
	})
}

func TestMockeyOpOption_ApplyOptsAutoUpdateEnabled(t *testing.T) {
	mockey.PatchConvey("applyOpts succeeds when auto update enabled with exit code", t, func() {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithEnableAutoUpdate(true),
			WithAutoUpdateExitCode(0),
		})
		require.NoError(t, err)
		assert.True(t, op.enableAutoUpdate)
		assert.Equal(t, 0, op.autoUpdateExitCode)
	})
}

// --- Body struct tests ---

func TestMockeyBody_Struct(t *testing.T) {
	mockey.PatchConvey("Body struct fields set correctly", t, func() {
		body := Body{
			Data:  []byte("test-data"),
			ReqID: "req-123",
		}
		assert.Equal(t, []byte("test-data"), body.Data)
		assert.Equal(t, "req-123", body.ReqID)
	})
}

func TestMockeyBody_EmptyStruct(t *testing.T) {
	mockey.PatchConvey("Body empty struct has zero values", t, func() {
		body := Body{}
		assert.Nil(t, body.Data)
		assert.Empty(t, body.ReqID)
	})
}

// --- Request/Response struct tests ---

func TestMockeyRequest_Struct(t *testing.T) {
	mockey.PatchConvey("Request struct fields set correctly", t, func() {
		req := Request{
			Method:        "states",
			Components:    []string{"comp1", "comp2"},
			UpdateVersion: "v1.0.0",
			ComponentName: "test-comp",
			Token:         "test-token",
		}
		assert.Equal(t, "states", req.Method)
		assert.Equal(t, []string{"comp1", "comp2"}, req.Components)
		assert.Equal(t, "v1.0.0", req.UpdateVersion)
		assert.Equal(t, "test-comp", req.ComponentName)
		assert.Equal(t, "test-token", req.Token)
	})
}

func TestMockeyResponse_Struct(t *testing.T) {
	mockey.PatchConvey("Response struct fields set correctly", t, func() {
		resp := Response{
			Error:     "test error",
			ErrorCode: http.StatusBadRequest,
			Token:     "resp-token",
		}
		assert.Equal(t, "test error", resp.Error)
		assert.Equal(t, int32(http.StatusBadRequest), resp.ErrorCode)
		assert.Equal(t, "resp-token", resp.Token)
	})
}

func TestMockeyBootstrapRequest_Struct(t *testing.T) {
	mockey.PatchConvey("BootstrapRequest struct fields set correctly", t, func() {
		br := BootstrapRequest{
			TimeoutInSeconds: 30,
			ScriptBase64:     base64.StdEncoding.EncodeToString([]byte("echo hello")),
		}
		assert.Equal(t, 30, br.TimeoutInSeconds)
		decoded, err := base64.StdEncoding.DecodeString(br.ScriptBase64)
		require.NoError(t, err)
		assert.Equal(t, "echo hello", string(decoded))
	})
}

func TestMockeyBootstrapResponse_Struct(t *testing.T) {
	mockey.PatchConvey("BootstrapResponse struct fields set correctly", t, func() {
		br := BootstrapResponse{
			Output:   "hello\n",
			ExitCode: 0,
		}
		assert.Equal(t, "hello\n", br.Output)
		assert.Equal(t, int32(0), br.ExitCode)
	})
}

// --- processGossip tests ---

func TestMockeyProcessGossip_NilFunc(t *testing.T) {
	mockey.PatchConvey("processGossip returns early when createGossipRequestFunc is nil", t, func() {
		s := &Session{
			createGossipRequestFunc: nil,
		}
		resp := &Response{}
		s.processGossip(resp)
		assert.Nil(t, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessGossip_Success(t *testing.T) {
	mockey.PatchConvey("processGossip sets gossip request on success", t, func() {
		expectedGossipReq := &apiv1.GossipRequest{
			MachineID: "machine-abc",
		}
		s := &Session{
			machineID: "machine-abc",
			createGossipRequestFunc: func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
				return expectedGossipReq, nil
			},
		}
		resp := &Response{}
		s.processGossip(resp)
		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessGossip_Error(t *testing.T) {
	mockey.PatchConvey("processGossip sets error on failure", t, func() {
		s := &Session{
			machineID: "machine-abc",
			createGossipRequestFunc: func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
				return nil, errors.New("gossip creation failed")
			},
		}
		resp := &Response{}
		s.processGossip(resp)
		assert.Nil(t, resp.GossipRequest)
		assert.Equal(t, "gossip creation failed", resp.Error)
	})
}

func TestMockeyProcessGossip_WithNvmlInstance(t *testing.T) {
	mockey.PatchConvey("processGossip passes nvml instance to create func", t, func() {
		inst := &mockNvmlInstanceForMockey{}
		var receivedInst nvidianvml.Instance

		expectedGossipReq := &apiv1.GossipRequest{MachineID: "machine-xyz"}
		s := &Session{
			machineID:    "machine-xyz",
			nvmlInstance: inst,
			createGossipRequestFunc: func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
				receivedInst = nvmlInstance
				return expectedGossipReq, nil
			},
		}
		resp := &Response{}
		s.processGossip(resp)
		assert.Equal(t, inst, receivedInst)
		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
	})
}

// --- processSetPluginSpecs tests ---

func TestMockeyProcessSetPluginSpecs_NilSaveFunc(t *testing.T) {
	mockey.PatchConvey("processSetPluginSpecs returns error when save func is nil", t, func() {
		s := &Session{
			savePluginSpecsFunc: nil,
		}
		resp := &Response{}
		specs := pkgcustomplugins.Specs{
			{PluginName: "test-plugin", PluginType: pkgcustomplugins.SpecTypeComponent},
		}
		exitCode := s.processSetPluginSpecs(context.Background(), resp, specs)
		assert.Nil(t, exitCode)
		assert.Equal(t, "save plugin specs function is not initialized", resp.Error)
	})
}

func TestMockeyProcessSetPluginSpecs_SaveError(t *testing.T) {
	mockey.PatchConvey("processSetPluginSpecs returns error when save fails", t, func() {
		s := &Session{
			savePluginSpecsFunc: func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
				return false, errors.New("save failed")
			},
		}
		resp := &Response{}
		specs := pkgcustomplugins.Specs{
			{PluginName: "test-plugin"},
		}
		exitCode := s.processSetPluginSpecs(context.Background(), resp, specs)
		assert.Nil(t, exitCode)
		assert.Equal(t, "save failed", resp.Error)
	})
}

func TestMockeyProcessSetPluginSpecs_NoUpdate(t *testing.T) {
	mockey.PatchConvey("processSetPluginSpecs returns nil exit code when no update", t, func() {
		s := &Session{
			savePluginSpecsFunc: func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
				return false, nil
			},
		}
		resp := &Response{}
		specs := pkgcustomplugins.Specs{
			{PluginName: "test-plugin"},
		}
		exitCode := s.processSetPluginSpecs(context.Background(), resp, specs)
		assert.Nil(t, exitCode)
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessSetPluginSpecs_Updated(t *testing.T) {
	mockey.PatchConvey("processSetPluginSpecs returns exit code 0 when updated", t, func() {
		s := &Session{
			savePluginSpecsFunc: func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
				return true, nil
			},
		}
		resp := &Response{}
		specs := pkgcustomplugins.Specs{
			{PluginName: "test-plugin"},
		}
		exitCode := s.processSetPluginSpecs(context.Background(), resp, specs)
		require.NotNil(t, exitCode)
		assert.Equal(t, 0, *exitCode)
		assert.Empty(t, resp.Error)
	})
}

// --- processGetPluginSpecs tests ---

func TestMockeyProcessGetPluginSpecs_NoCustomPlugins(t *testing.T) {
	mockey.PatchConvey("processGetPluginSpecs returns empty specs when no custom plugins", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		registry.On("All").Return([]components.Component{comp})

		s := &Session{
			componentsRegistry: registry,
		}
		resp := &Response{}
		s.processGetPluginSpecs(resp)

		// mockComponent does not implement CustomPluginRegisteree, so specs should be empty
		assert.Empty(t, resp.CustomPluginSpecs)
		assert.Empty(t, resp.Error)
		registry.AssertExpectations(t)
	})
}

func TestMockeyProcessGetPluginSpecs_EmptyRegistry(t *testing.T) {
	mockey.PatchConvey("processGetPluginSpecs returns empty specs with empty registry", t, func() {
		registry := new(mockComponentRegistry)
		registry.On("All").Return([]components.Component{})

		s := &Session{
			componentsRegistry: registry,
		}
		resp := &Response{}
		s.processGetPluginSpecs(resp)

		assert.Empty(t, resp.CustomPluginSpecs)
		assert.Empty(t, resp.Error)
		registry.AssertExpectations(t)
	})
}

// --- processBootstrap tests ---

func TestMockeyProcessBootstrap_NilRequest(t *testing.T) {
	mockey.PatchConvey("processBootstrap returns early when bootstrap is nil", t, func() {
		s := &Session{}
		ctx := context.Background()
		payload := Request{Bootstrap: nil}
		resp := &Response{}

		s.processBootstrap(ctx, payload, resp)

		assert.Nil(t, resp.Bootstrap)
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessBootstrap_InvalidBase64(t *testing.T) {
	mockey.PatchConvey("processBootstrap fails on invalid base64 input", t, func() {
		s := &Session{}
		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64: "!invalid-base64###",
			},
		}
		resp := &Response{}

		s.processBootstrap(ctx, payload, resp)

		assert.Nil(t, resp.Bootstrap)
		assert.NotEmpty(t, resp.Error)
		assert.Contains(t, resp.Error, "illegal base64")
	})
}

func TestMockeyProcessBootstrap_Success(t *testing.T) {
	mockey.PatchConvey("processBootstrap executes script successfully", t, func() {
		runner := new(mockProcessRunner)
		runner.On("RunUntilCompletion", mock.Anything, "echo hello").Return([]byte("hello\n"), 0, nil)

		s := &Session{
			processRunner: runner,
		}
		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64:     base64.StdEncoding.EncodeToString([]byte("echo hello")),
				TimeoutInSeconds: 5,
			},
		}
		resp := &Response{}

		s.processBootstrap(ctx, payload, resp)

		require.NotNil(t, resp.Bootstrap)
		assert.Equal(t, "hello\n", resp.Bootstrap.Output)
		assert.Equal(t, int32(0), resp.Bootstrap.ExitCode)
		assert.Empty(t, resp.Error)
		runner.AssertExpectations(t)
	})
}

func TestMockeyProcessBootstrap_DefaultTimeout(t *testing.T) {
	mockey.PatchConvey("processBootstrap uses default 10s timeout when zero", t, func() {
		runner := new(mockProcessRunner)
		runner.On("RunUntilCompletion", mock.Anything, "sleep 1").Return([]byte(""), 0, nil)

		s := &Session{
			processRunner: runner,
		}
		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64:     base64.StdEncoding.EncodeToString([]byte("sleep 1")),
				TimeoutInSeconds: 0,
			},
		}
		resp := &Response{}

		s.processBootstrap(ctx, payload, resp)

		require.NotNil(t, resp.Bootstrap)
		assert.Empty(t, resp.Error)
		runner.AssertExpectations(t)
	})
}

func TestMockeyProcessBootstrap_RunError(t *testing.T) {
	mockey.PatchConvey("processBootstrap sets error when script execution fails", t, func() {
		runner := new(mockProcessRunner)
		runner.On("RunUntilCompletion", mock.Anything, "exit 1").Return([]byte(""), 1, errors.New("command failed"))

		s := &Session{
			processRunner: runner,
		}
		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64:     base64.StdEncoding.EncodeToString([]byte("exit 1")),
				TimeoutInSeconds: 5,
			},
		}
		resp := &Response{}

		s.processBootstrap(ctx, payload, resp)

		require.NotNil(t, resp.Bootstrap)
		assert.Equal(t, int32(1), resp.Bootstrap.ExitCode)
		assert.Equal(t, "command failed", resp.Error)
		runner.AssertExpectations(t)
	})
}

// --- processUpdate tests ---

func TestMockeyProcessUpdate_AutoUpdateDisabled(t *testing.T) {
	mockey.PatchConvey("processUpdate returns error when auto update is disabled", t, func() {
		s := &Session{
			enableAutoUpdate: false,
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: "v2.0.0"}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Equal(t, "auto update is disabled", resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessUpdate_EmptyVersion(t *testing.T) {
	mockey.PatchConvey("processUpdate returns error for empty version", t, func() {
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return true, nil
		}).Build()

		s := &Session{
			enableAutoUpdate:   true,
			autoUpdateExitCode: 0,
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: ""}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Equal(t, "update_version is empty", resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessUpdate_NotSystemdManaged(t *testing.T) {
	mockey.PatchConvey("processUpdate returns error when not systemd managed and no exit code", t, func() {
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		s := &Session{
			enableAutoUpdate:   true,
			autoUpdateExitCode: -1, // not set
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: "v2.0.0"}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Equal(t, "gpud is not managed with systemd", resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessUpdate_UpdateExecutableError(t *testing.T) {
	mockey.PatchConvey("processUpdate sets error when UpdateExecutable fails", t, func() {
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		mockey.Mock(update.UpdateExecutable).To(func(version, url string, systemdManaged bool) error {
			return errors.New("update failed: download error")
		}).Build()

		s := &Session{
			enableAutoUpdate:   true,
			autoUpdateExitCode: 0, // exit code set, bypass systemd check
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: "v2.0.0"}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Equal(t, "update failed: download error", resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessUpdate_UpdateExecutableSuccess(t *testing.T) {
	mockey.PatchConvey("processUpdate sets exit code on successful update", t, func() {
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		mockey.Mock(update.UpdateExecutable).To(func(version, url string, systemdManaged bool) error {
			return nil
		}).Build()

		s := &Session{
			enableAutoUpdate:   true,
			autoUpdateExitCode: 42,
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: "v3.0.0"}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 42, restartExitCode)
	})
}

func TestMockeyProcessUpdate_PackageUpdate(t *testing.T) {
	mockey.PatchConvey("processUpdate handles package:version format", t, func() {
		var receivedPkg, receivedVersion string
		mockey.Mock(update.PackageUpdate).To(func(pkg, version, url, dataDir string) error {
			receivedPkg = pkg
			receivedVersion = version
			return nil
		}).Build()

		s := &Session{
			dataDir: "/tmp/test",
		}
		ctx := context.Background()
		payload := Request{UpdateVersion: "mypackage:v1.0.0"}
		resp := &Response{}
		restartExitCode := -1

		s.processUpdate(ctx, payload, resp, &restartExitCode)

		assert.Equal(t, "mypackage", receivedPkg)
		assert.Equal(t, "v1.0.0", receivedVersion)
		assert.Equal(t, -1, restartExitCode) // package update does not set exit code
	})
}

// --- getEvents tests ---

func TestMockeyGetEvents_MismatchMethod(t *testing.T) {
	mockey.PatchConvey("getEvents returns error for mismatched method", t, func() {
		s := &Session{}
		payload := Request{Method: "not_events"}

		result, err := s.getEvents(context.Background(), payload)

		require.Error(t, err)
		assert.Equal(t, "mismatch method", err.Error())
		assert.Nil(t, result)
	})
}

func TestMockeyGetEvents_UsesDefaultComponents(t *testing.T) {
	mockey.PatchConvey("getEvents uses default components when none specified", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		events := apiv1.Events{
			{Name: "event1"},
		}
		registry.On("Get", "comp1").Return(comp)
		comp.On("Events", mock.Anything, mock.Anything).Return(events, nil)

		s := &Session{
			componentsRegistry: registry,
			components:         []string{"comp1"},
		}
		payload := Request{Method: "events"}

		result, err := s.getEvents(context.Background(), payload)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "comp1", result[0].Component)
		assert.Equal(t, events, result[0].Events)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

func TestMockeyGetEvents_UsesSpecifiedComponents(t *testing.T) {
	mockey.PatchConvey("getEvents uses specified components from payload", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		events := apiv1.Events{
			{Name: "specific-event"},
		}
		registry.On("Get", "specified-comp").Return(comp)
		comp.On("Events", mock.Anything, mock.Anything).Return(events, nil)

		s := &Session{
			componentsRegistry: registry,
			components:         []string{"default-comp"},
		}
		payload := Request{
			Method:     "events",
			Components: []string{"specified-comp"},
		}

		result, err := s.getEvents(context.Background(), payload)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "specified-comp", result[0].Component)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

func TestMockeyGetEvents_ComponentNotFound(t *testing.T) {
	mockey.PatchConvey("getEvents handles component not found gracefully", t, func() {
		registry := new(mockComponentRegistry)
		registry.On("Get", "missing").Return(nil)

		s := &Session{
			componentsRegistry: registry,
			components:         []string{"missing"},
		}
		payload := Request{Method: "events"}

		result, err := s.getEvents(context.Background(), payload)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "missing", result[0].Component)
		assert.Nil(t, result[0].Events)
		registry.AssertExpectations(t)
	})
}

func TestMockeyGetEvents_ComponentEventError(t *testing.T) {
	mockey.PatchConvey("getEvents handles component event error", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		registry.On("Get", "error-comp").Return(comp)
		comp.On("Events", mock.Anything, mock.Anything).Return(apiv1.Events{}, errors.New("events error"))

		s := &Session{
			componentsRegistry: registry,
			components:         []string{"error-comp"},
		}
		payload := Request{Method: "events"}

		result, err := s.getEvents(context.Background(), payload)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "error-comp", result[0].Component)
		// Events are nil when there is an error retrieving them
		assert.Nil(t, result[0].Events)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

// --- getHealthStates tests ---

func TestMockeyGetHealthStates_MismatchMethod(t *testing.T) {
	mockey.PatchConvey("getHealthStates returns error for mismatched method", t, func() {
		s := &Session{}
		payload := Request{Method: "not_states"}

		result, err := s.getHealthStates(payload)

		require.Error(t, err)
		assert.Equal(t, "mismatch method", err.Error())
		assert.Nil(t, result)
	})
}

func TestMockeyGetHealthStates_ComponentNotFound(t *testing.T) {
	mockey.PatchConvey("getHealthStatesFromComponent returns empty states for missing component", t, func() {
		registry := new(mockComponentRegistry)
		registry.On("Get", "nonexistent").Return(nil)

		s := &Session{
			componentsRegistry: registry,
		}

		result := s.getHealthStatesFromComponent("nonexistent", time.Time{})

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.States)
		registry.AssertExpectations(t)
	})
}

func TestMockeyGetHealthStates_ComponentFound(t *testing.T) {
	mockey.PatchConvey("getHealthStatesFromComponent returns states for found component", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "gpu-health"},
		}
		registry.On("Get", "gpu").Return(comp)
		comp.On("LastHealthStates").Return(healthStates)

		s := &Session{
			componentsRegistry: registry,
		}

		rebootTime := time.Now().Add(-10 * time.Minute)
		result := s.getHealthStatesFromComponent("gpu", rebootTime)

		assert.Equal(t, "gpu", result.Component)
		assert.Equal(t, healthStates, result.States)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

func TestMockeyGetHealthStatesFromComponentWithDeps_Initializing(t *testing.T) {
	mockey.PatchConvey("getHealthStatesFromComponentWithDeps sets initializing during grace period", t, func() {
		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeDegraded, Name: "degraded-state"},
		}
		comp.On("LastHealthStates").Return(healthStates)

		getComp := func(name string) components.Component {
			if name == "test-comp" {
				return comp
			}
			return nil
		}

		// Use a very recent reboot time (within 5 min grace period)
		rebootTime := time.Now().Add(-1 * time.Minute)
		result := getHealthStatesFromComponentWithDeps("test-comp", rebootTime, getComp)

		assert.Equal(t, "test-comp", result.Component)
		require.Len(t, result.States, 1)
		// Unhealthy state should be set to initializing during grace period
		assert.Equal(t, apiv1.HealthStateTypeInitializing, result.States[0].Health)
	})
}

func TestMockeyGetHealthStatesFromComponentWithDeps_HealthyNotModified(t *testing.T) {
	mockey.PatchConvey("getHealthStatesFromComponentWithDeps does not modify healthy state", t, func() {
		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "healthy-state"},
		}
		comp.On("LastHealthStates").Return(healthStates)

		getComp := func(name string) components.Component {
			if name == "test-comp" {
				return comp
			}
			return nil
		}

		rebootTime := time.Now().Add(-1 * time.Minute)
		result := getHealthStatesFromComponentWithDeps("test-comp", rebootTime, getComp)

		assert.Equal(t, "test-comp", result.Component)
		require.Len(t, result.States, 1)
		// Healthy state should remain unchanged
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.States[0].Health)
	})
}

// --- getMetrics tests ---

func TestMockeyGetMetrics_MismatchMethod(t *testing.T) {
	mockey.PatchConvey("getMetrics returns error for mismatched method", t, func() {
		s := &Session{}
		payload := Request{Method: "not_metrics"}

		result, err := s.getMetrics(context.Background(), payload)

		require.Error(t, err)
		assert.Equal(t, "mismatch method", err.Error())
		assert.Nil(t, result)
	})
}

func TestMockeyGetMetrics_ComponentNotFound(t *testing.T) {
	mockey.PatchConvey("getMetricsFromComponent returns empty metrics for missing component", t, func() {
		registry := new(mockComponentRegistry)
		registry.On("Get", "missing").Return(nil)

		metricsStore := new(mockMetricsStore)
		s := &Session{
			componentsRegistry: registry,
			metricsStore:       metricsStore,
		}

		since := time.Now().Add(-time.Hour)
		result := s.getMetricsFromComponent(context.Background(), "missing", since)

		assert.Equal(t, "missing", result.Component)
		assert.Empty(t, result.Metrics)
		registry.AssertExpectations(t)
	})
}

func TestMockeyGetMetrics_Success(t *testing.T) {
	mockey.PatchConvey("getMetricsFromComponent returns metrics for found component", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		metricsStore := new(mockMetricsStore)

		metricsData := pkgmetrics.Metrics{
			{Name: "cpu_usage", Value: 75.5, UnixMilliseconds: 1000, Component: "cpu"},
		}

		registry.On("Get", "cpu").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		s := &Session{
			componentsRegistry: registry,
			metricsStore:       metricsStore,
		}

		since := time.Now().Add(-time.Hour)
		result := s.getMetricsFromComponent(context.Background(), "cpu", since)

		assert.Equal(t, "cpu", result.Component)
		require.Len(t, result.Metrics, 1)
		assert.Equal(t, "cpu_usage", result.Metrics[0].Name)
		assert.Equal(t, 75.5, result.Metrics[0].Value)
		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

func TestMockeyGetMetrics_StoreError(t *testing.T) {
	mockey.PatchConvey("getMetricsFromComponent handles store read error", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		metricsStore := new(mockMetricsStore)

		registry.On("Get", "disk").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(pkgmetrics.Metrics{}, errors.New("store read error"))

		s := &Session{
			componentsRegistry: registry,
			metricsStore:       metricsStore,
		}

		since := time.Now().Add(-time.Hour)
		result := s.getMetricsFromComponent(context.Background(), "disk", since)

		assert.Equal(t, "disk", result.Component)
		assert.Empty(t, result.Metrics)
		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

func TestMockeyGetMetrics_WithCustomSince(t *testing.T) {
	mockey.PatchConvey("getMetrics uses custom since duration from payload", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		metricsStore := new(mockMetricsStore)

		metricsData := pkgmetrics.Metrics{
			{Name: "metric1", Value: 42.0, UnixMilliseconds: 1000, Component: "comp1"},
		}

		registry.On("Get", "comp1").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		s := &Session{
			componentsRegistry: registry,
			metricsStore:       metricsStore,
			components:         []string{"comp1"},
		}

		payload := Request{
			Method: "metrics",
			Since:  2 * time.Hour,
		}

		result, err := s.getMetrics(context.Background(), payload)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

// --- Token management tests ---

func TestMockeyProcessUpdateToken_EmptyToken(t *testing.T) {
	mockey.PatchConvey("processUpdateToken returns error for empty token", t, func() {
		s := &Session{}
		payload := Request{Token: ""}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Equal(t, "token cannot be empty", resp.Error)
	})
}

func TestMockeyProcessUpdateToken_SameToken(t *testing.T) {
	mockey.PatchConvey("processUpdateToken returns early when token matches cache", t, func() {
		s := &Session{
			token: "existing-token",
		}
		payload := Request{Token: "existing-token"}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessUpdateToken_NilDB(t *testing.T) {
	mockey.PatchConvey("processUpdateToken returns error when DB is nil", t, func() {
		s := &Session{
			token: "old-token",
			dbRW:  nil,
		}
		payload := Request{Token: "new-token"}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Equal(t, "database connection not available", resp.Error)
	})
}

func TestMockeyProcessUpdateToken_HealthCheckFails(t *testing.T) {
	mockey.PatchConvey("processUpdateToken returns error when health check fails", t, func() {
		dbRW := &sql.DB{} // dummy db (won't be used since health check fails first)

		s := &Session{
			token: "old-token",
			dbRW:  dbRW,
			checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return errors.New("health check failed with new token")
			},
		}
		payload := Request{Token: "bad-new-token"}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Contains(t, resp.Error, "token validation failed")
		assert.Contains(t, resp.Error, "health check failed with new token")
	})
}

func TestMockeyProcessUpdateToken_SetMetadataFails(t *testing.T) {
	mockey.PatchConvey("processUpdateToken returns error when metadata update fails", t, func() {
		dbRW := &sql.DB{} // dummy

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key string, value string) error {
			return errors.New("db write error")
		}).Build()

		s := &Session{
			token: "old-token",
			dbRW:  dbRW,
			checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return nil // health check passes
			},
			closer: &closeOnce{closer: make(chan any)},
		}
		payload := Request{Token: "new-token"}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Equal(t, "db write error", resp.Error)
	})
}

func TestMockeyProcessUpdateToken_Success(t *testing.T) {
	mockey.PatchConvey("processUpdateToken succeeds and updates token", t, func() {
		dbRW := &sql.DB{} // dummy

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key string, value string) error {
			return nil
		}).Build()

		s := &Session{
			token: "old-token",
			dbRW:  dbRW,
			checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return nil
			},
			closer: &closeOnce{closer: make(chan any)},
		}
		payload := Request{Token: "new-token"}
		resp := &Response{}

		s.processUpdateToken(payload, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, "new-token", s.getToken())
	})
}

func TestMockeyProcessGetToken_NilDB(t *testing.T) {
	mockey.PatchConvey("processGetToken returns error when DB is nil", t, func() {
		s := &Session{
			dbRO: nil,
		}
		resp := &Response{}

		s.processGetToken(resp)

		assert.Equal(t, "database connection not available", resp.Error)
	})
}

func TestMockeyProcessGetToken_CachedToken(t *testing.T) {
	mockey.PatchConvey("processGetToken returns cached token when available", t, func() {
		s := &Session{
			token: "cached-token",
			dbRO:  &sql.DB{}, // dummy
		}
		resp := &Response{}

		s.processGetToken(resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, "cached-token", resp.Token)
	})
}

func TestMockeyProcessGetToken_ReadFromDB(t *testing.T) {
	mockey.PatchConvey("processGetToken reads token from database when cache empty", t, func() {
		dbRO := &sql.DB{} // dummy

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "db-token-value", nil
		}).Build()

		s := &Session{
			token: "", // empty cache
			dbRO:  dbRO,
		}
		resp := &Response{}

		s.processGetToken(resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, "db-token-value", resp.Token)
		assert.Equal(t, "db-token-value", s.getToken()) // should update cache
	})
}

func TestMockeyProcessGetToken_ReadFromDBError(t *testing.T) {
	mockey.PatchConvey("processGetToken returns error when DB read fails", t, func() {
		dbRO := &sql.DB{} // dummy

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("db read error")
		}).Build()

		s := &Session{
			token: "",
			dbRO:  dbRO,
		}
		resp := &Response{}

		s.processGetToken(resp)

		assert.Equal(t, "db read error", resp.Error)
		assert.Empty(t, resp.Token)
	})
}

// --- Token get/set concurrency tests ---

func TestMockeyTokenGetSet(t *testing.T) {
	mockey.PatchConvey("setToken and getToken work correctly", t, func() {
		s := &Session{}
		assert.Empty(t, s.getToken())

		s.setToken("token-abc")
		assert.Equal(t, "token-abc", s.getToken())

		s.setToken("token-xyz")
		assert.Equal(t, "token-xyz", s.getToken())
	})
}

// --- LastPackageTimestamp tests ---

func TestMockeyLastPackageTimestamp(t *testing.T) {
	mockey.PatchConvey("setLastPackageTimestamp and getLastPackageTimestamp work", t, func() {
		s := &Session{}
		assert.True(t, s.getLastPackageTimestamp().IsZero())

		now := time.Now()
		s.setLastPackageTimestamp(now)
		assert.True(t, s.getLastPackageTimestamp().Equal(now))
	})
}

// --- closeOnce tests ---

func TestMockeyCloseOnce(t *testing.T) {
	mockey.PatchConvey("closeOnce only closes channel once", t, func() {
		c := &closeOnce{closer: make(chan any)}

		c.Close()
		select {
		case <-c.Done():
			// expected
		default:
			assert.Fail(t, "channel should be closed after Close()")
		}

		// Second close should not panic
		c.Close()

		select {
		case <-c.Done():
			// expected, still closed
		default:
			assert.Fail(t, "channel should still be closed")
		}
	})
}

// --- processUpdateConfig tests ---

func TestMockeyProcessUpdateConfig_EmptyMap(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig returns early for empty config map", t, func() {
		called := false
		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {
				called = true
			},
		}
		resp := &Response{}

		s.processUpdateConfig(map[string]string{}, resp)

		assert.Empty(t, resp.Error)
		assert.False(t, called, "should not call any setter for empty map")
	})
}

func TestMockeyProcessUpdateConfig_InfinibandConfig(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig handles valid infiniband config", t, func() {
		var receivedStates componentsnvidiainfinibanditypes.ExpectedPortStates
		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {
				receivedStates = states
			},
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {},
			setDefaultGPUCountsFunc:                func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {},
			setDefaultNFSGroupConfigsFunc:          func(cfgs pkgnfschecker.Configs) {},
			setDefaultXIDRebootThresholdFunc:       func(threshold componentsxid.RebootThreshold) {},
			setDefaultTemperatureThresholdsFunc:    func(thresholds componentstemperature.Thresholds) {},
		}

		configMap := map[string]string{
			"accelerator-nvidia-infiniband": `{"at_least_ports": 4, "at_least_rate": 200}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 4, receivedStates.AtLeastPorts)
		assert.Equal(t, 200, receivedStates.AtLeastRate)
	})
}

func TestMockeyProcessUpdateConfig_InvalidJSON(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig returns error for invalid JSON", t, func() {
		s := &Session{
			setDefaultIbExpectedPortStatesFunc: func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {},
		}
		configMap := map[string]string{
			"accelerator-nvidia-infiniband": `{"at_least_ports":}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Contains(t, resp.Error, "invalid character")
	})
}

func TestMockeyProcessUpdateConfig_GPUCounts(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig handles valid GPU counts config", t, func() {
		var receivedCounts componentsnvidiagpucounts.ExpectedGPUCounts
		s := &Session{
			setDefaultIbExpectedPortStatesFunc:     func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {},
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {},
			setDefaultGPUCountsFunc: func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {
				receivedCounts = counts
			},
			setDefaultNFSGroupConfigsFunc:       func(cfgs pkgnfschecker.Configs) {},
			setDefaultXIDRebootThresholdFunc:    func(threshold componentsxid.RebootThreshold) {},
			setDefaultTemperatureThresholdsFunc: func(thresholds componentstemperature.Thresholds) {},
		}

		configMap := map[string]string{
			"accelerator-nvidia-gpu-counts": `{"count": 8}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 8, receivedCounts.Count)
	})
}

func TestMockeyProcessUpdateConfig_XIDConfig(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig handles valid XID config", t, func() {
		var receivedThreshold componentsxid.RebootThreshold
		s := &Session{
			setDefaultIbExpectedPortStatesFunc:     func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {},
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {},
			setDefaultGPUCountsFunc:                func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {},
			setDefaultNFSGroupConfigsFunc:          func(cfgs pkgnfschecker.Configs) {},
			setDefaultXIDRebootThresholdFunc: func(threshold componentsxid.RebootThreshold) {
				receivedThreshold = threshold
			},
			setDefaultTemperatureThresholdsFunc: func(thresholds componentstemperature.Thresholds) {},
		}

		configMap := map[string]string{
			"accelerator-nvidia-error-xid": `{"threshold": 10}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, 10, receivedThreshold.Threshold)
	})
}

func TestMockeyProcessUpdateConfig_TemperatureConfig(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig handles valid temperature config", t, func() {
		var receivedThresholds componentstemperature.Thresholds
		s := &Session{
			setDefaultIbExpectedPortStatesFunc:     func(states componentsnvidiainfinibanditypes.ExpectedPortStates) {},
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {},
			setDefaultGPUCountsFunc:                func(counts componentsnvidiagpucounts.ExpectedGPUCounts) {},
			setDefaultNFSGroupConfigsFunc:          func(cfgs pkgnfschecker.Configs) {},
			setDefaultXIDRebootThresholdFunc:       func(threshold componentsxid.RebootThreshold) {},
			setDefaultTemperatureThresholdsFunc: func(thresholds componentstemperature.Thresholds) {
				receivedThresholds = thresholds
			},
		}

		configMap := map[string]string{
			"accelerator-nvidia-temperature": `{"celsius_slowdown_margin": 15}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
		assert.Equal(t, int32(15), receivedThresholds.CelsiusSlowdownMargin)
	})
}

func TestMockeyProcessUpdateConfig_NilFuncs(t *testing.T) {
	mockey.PatchConvey("processUpdateConfig handles nil setter functions", t, func() {
		s := &Session{
			setDefaultIbExpectedPortStatesFunc:     nil,
			setDefaultNVLinkExpectedLinkStatesFunc: nil,
			setDefaultGPUCountsFunc:                nil,
			setDefaultNFSGroupConfigsFunc:          nil,
			setDefaultXIDRebootThresholdFunc:       nil,
			setDefaultTemperatureThresholdsFunc:    nil,
		}

		configMap := map[string]string{
			"accelerator-nvidia-infiniband": `{"at_least_ports": 2}`,
		}
		resp := &Response{}

		s.processUpdateConfig(configMap, resp)

		assert.Empty(t, resp.Error)
	})
}

// --- processRequest routing tests ---

func TestMockeyProcessRequest_UnknownMethod(t *testing.T) {
	mockey.PatchConvey("processRequest handles unknown method without error", t, func() {
		s := &Session{
			ctx: context.Background(),
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req1", Request{Method: "unknownXYZ"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessRequest_MetricsMethod(t *testing.T) {
	mockey.PatchConvey("processRequest routes metrics method correctly", t, func() {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)

		comp := new(mockComponent)
		registry.On("Get", "comp1").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(pkgmetrics.Metrics{}, nil)

		s := &Session{
			ctx:                context.Background(),
			componentsRegistry: registry,
			metricsStore:       metricsStore,
			components:         []string{"comp1"},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req2", Request{Method: "metrics"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.NotNil(t, resp.Metrics)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessRequest_StatesMethod(t *testing.T) {
	mockey.PatchConvey("processRequest routes states method correctly", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		}

		registry.On("Get", "comp1").Return(comp)
		comp.On("LastHealthStates").Return(healthStates)

		s := &Session{
			ctx:                context.Background(),
			componentsRegistry: registry,
			components:         []string{"comp1"},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req3", Request{Method: "states"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.NotNil(t, resp.States)
	})
}

func TestMockeyProcessRequest_EventsMethod(t *testing.T) {
	mockey.PatchConvey("processRequest routes events method correctly", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		events := apiv1.Events{}

		registry.On("Get", "comp1").Return(comp)
		comp.On("Events", mock.Anything, mock.Anything).Return(events, nil)

		s := &Session{
			ctx:                context.Background(),
			componentsRegistry: registry,
			components:         []string{"comp1"},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req4", Request{Method: "events"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.NotNil(t, resp.Events)
	})
}

func TestMockeyProcessRequest_UpdateDisabled(t *testing.T) {
	mockey.PatchConvey("processRequest handles update method with auto update disabled", t, func() {
		s := &Session{
			ctx:              context.Background(),
			enableAutoUpdate: false,
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req5",
			Request{Method: "update", UpdateVersion: "v2.0.0"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Equal(t, "auto update is disabled", resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessRequest_UpdateConfigSkipped(t *testing.T) {
	mockey.PatchConvey("processRequest skips updateConfig when skip flag is set", t, func() {
		called := false
		s := &Session{
			ctx:              context.Background(),
			skipUpdateConfig: true,
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {
				called = true
			},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req6",
			Request{Method: "updateConfig", UpdateConfig: map[string]string{"accelerator-nvidia-nvlink": `{}`}},
			resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.False(t, called, "updateConfig should be skipped when flag is set")
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessRequest_UpdateConfigProcessed(t *testing.T) {
	mockey.PatchConvey("processRequest processes updateConfig when skip flag is not set", t, func() {
		called := false
		s := &Session{
			ctx:              context.Background(),
			skipUpdateConfig: false,
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {
				called = true
			},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req7",
			Request{Method: "updateConfig", UpdateConfig: map[string]string{componentsnvidianvlink.Name: `{}`}},
			resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.True(t, called, "updateConfig should be processed when not skipped")
		assert.Empty(t, resp.Error)
	})
}

func TestMockeyProcessRequest_GossipAsync(t *testing.T) {
	mockey.PatchConvey("processRequest handles gossip method asynchronously", t, func() {
		s := &Session{
			ctx:         context.Background(),
			auditLogger: log.NewNopAuditLogger(),
			writer:      make(chan Body, 10),
			createGossipRequestFunc: func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
				return &apiv1.GossipRequest{MachineID: "test"}, nil
			},
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-gossip",
			Request{Method: "gossip"}, resp, &restartExitCode)

		assert.True(t, handledAsync, "gossip should be handled asynchronously")

		// Wait for async response
		select {
		case body := <-s.writer:
			assert.Equal(t, "req-gossip", body.ReqID)
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timeout waiting for async gossip response")
		}
	})
}

func TestMockeyProcessRequest_TriggerComponentAsync(t *testing.T) {
	mockey.PatchConvey("processRequest handles triggerComponent asynchronously", t, func() {
		registry := new(mockComponentRegistry)
		comp := new(mockComponent)
		checkResult := new(mockCheckResult)

		registry.On("Get", "test-comp").Return(comp)
		comp.On("Check").Return(checkResult)
		checkResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		})
		checkResult.On("ComponentName").Return("test-comp")

		s := &Session{
			ctx:                context.Background(),
			auditLogger:        log.NewNopAuditLogger(),
			writer:             make(chan Body, 10),
			componentsRegistry: registry,
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-trigger",
			Request{Method: "triggerComponent", ComponentName: "test-comp"}, resp, &restartExitCode)

		assert.True(t, handledAsync, "triggerComponent should be handled asynchronously")

		select {
		case body := <-s.writer:
			assert.Equal(t, "req-trigger", body.ReqID)
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timeout waiting for async trigger response")
		}
	})
}

func TestMockeyProcessRequest_SetPluginSpecs(t *testing.T) {
	mockey.PatchConvey("processRequest handles setPluginSpecs method", t, func() {
		saveCalled := false
		s := &Session{
			ctx: context.Background(),
			savePluginSpecsFunc: func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
				saveCalled = true
				return false, nil
			},
		}
		resp := &Response{}
		restartExitCode := -1

		specs := pkgcustomplugins.Specs{
			{PluginName: "test-plugin"},
		}
		handledAsync := s.processRequest(context.Background(), "req-set-plugins",
			Request{Method: "setPluginSpecs", CustomPluginSpecs: specs}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.True(t, saveCalled)
		assert.Empty(t, resp.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}

func TestMockeyProcessRequest_GetPluginSpecs(t *testing.T) {
	mockey.PatchConvey("processRequest handles getPluginSpecs method", t, func() {
		registry := new(mockComponentRegistry)
		registry.On("All").Return([]components.Component{})

		s := &Session{
			ctx:                context.Background(),
			componentsRegistry: registry,
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-get-plugins",
			Request{Method: "getPluginSpecs"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Empty(t, resp.Error)
		registry.AssertExpectations(t)
	})
}

func TestMockeyProcessRequest_UpdateToken(t *testing.T) {
	mockey.PatchConvey("processRequest handles updateToken method", t, func() {
		s := &Session{
			ctx:   context.Background(),
			token: "same-token",
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-update-token",
			Request{Method: "updateToken", Token: "same-token"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Empty(t, resp.Error) // same token, no update needed
	})
}

func TestMockeyProcessRequest_GetToken(t *testing.T) {
	mockey.PatchConvey("processRequest handles getToken method", t, func() {
		s := &Session{
			ctx:   context.Background(),
			token: "my-cached-token",
			dbRO:  &sql.DB{}, // dummy
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-get-token",
			Request{Method: "getToken"}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Empty(t, resp.Error)
		assert.Equal(t, "my-cached-token", resp.Token)
	})
}

func TestMockeyProcessRequest_Bootstrap(t *testing.T) {
	mockey.PatchConvey("processRequest handles bootstrap with nil payload", t, func() {
		s := &Session{
			ctx: context.Background(),
		}
		resp := &Response{}
		restartExitCode := -1

		handledAsync := s.processRequest(context.Background(), "req-bootstrap",
			Request{Method: "bootstrap", Bootstrap: nil}, resp, &restartExitCode)

		assert.False(t, handledAsync)
		assert.Empty(t, resp.Error)
		assert.Nil(t, resp.Bootstrap)
	})
}

// --- processRequestAsync tests ---

func TestMockeyProcessRequestAsync_UnsupportedMethod_ResponseFields(t *testing.T) {
	mockey.PatchConvey("processRequestAsync returns error for unsupported method", t, func() {
		s := &Session{
			ctx:         context.Background(),
			auditLogger: log.NewNopAuditLogger(),
			writer:      make(chan Body, 10),
		}

		s.processRequestAsync("req-async-unknown", "unknownAsync", Request{Method: "unknownAsync"})

		select {
		case body := <-s.writer:
			assert.Equal(t, "req-async-unknown", body.ReqID)
		default:
			assert.Fail(t, "expected response to be sent")
		}
	})
}

func TestMockeyProcessRequestAsync_GossipSuccess(t *testing.T) {
	mockey.PatchConvey("processRequestAsync handles gossip successfully", t, func() {
		expectedReq := &apiv1.GossipRequest{MachineID: "machine-1"}

		s := &Session{
			ctx:         context.Background(),
			auditLogger: log.NewNopAuditLogger(),
			writer:      make(chan Body, 10),
			machineID:   "machine-1",
			createGossipRequestFunc: func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
				return expectedReq, nil
			},
		}

		s.processRequestAsync("req-gossip-async", "gossip", Request{Method: "gossip"})

		select {
		case body := <-s.writer:
			assert.Equal(t, "req-gossip-async", body.ReqID)
			assert.NotNil(t, body.Data)
		default:
			assert.Fail(t, "expected response to be sent")
		}
	})
}

// --- sendResponse tests ---

func TestMockeySendResponse_Success(t *testing.T) {
	mockey.PatchConvey("sendResponse sends response to writer channel", t, func() {
		s := &Session{
			ctx:         context.Background(),
			auditLogger: log.NewNopAuditLogger(),
			writer:      make(chan Body, 10),
		}

		resp := &Response{Error: ""}
		s.sendResponse("req-send", "testMethod", resp)

		select {
		case body := <-s.writer:
			assert.Equal(t, "req-send", body.ReqID)
			assert.NotNil(t, body.Data)
		default:
			assert.Fail(t, "expected response to be sent")
		}
	})
}

func TestMockeySendResponse_WithError(t *testing.T) {
	mockey.PatchConvey("sendResponse sends error response to writer channel", t, func() {
		s := &Session{
			ctx:         context.Background(),
			auditLogger: log.NewNopAuditLogger(),
			writer:      make(chan Body, 10),
		}

		resp := &Response{Error: "test error message"}
		s.sendResponse("req-err", "failMethod", resp)

		select {
		case body := <-s.writer:
			assert.Equal(t, "req-err", body.ReqID)
			assert.Contains(t, string(body.Data), "test error message")
		default:
			assert.Fail(t, "expected response to be sent")
		}
	})
}

// --- trySendResponse tests ---

func TestMockeyTrySendResponse_Success(t *testing.T) {
	mockey.PatchConvey("trySendResponse returns true when write succeeds", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		s := &Session{
			ctx:    ctx,
			writer: make(chan Body, 10),
		}

		body := Body{ReqID: "req-try", Data: []byte("data")}
		sent := s.trySendResponse(body)
		assert.True(t, sent)

		select {
		case received := <-s.writer:
			assert.Equal(t, "req-try", received.ReqID)
		default:
			assert.Fail(t, "expected body in writer channel")
		}
	})
}

func TestMockeyTrySendResponse_ContextDone(t *testing.T) {
	mockey.PatchConvey("trySendResponse returns false when context is done", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		// Use unbuffered channel so the select statement cannot pick the write case
		s := &Session{
			ctx:    ctx,
			writer: make(chan Body),
		}

		body := Body{ReqID: "req-try-cancel"}
		sent := s.trySendResponse(body)
		assert.False(t, sent)
	})
}

// --- drainReaderChannel tests ---

func TestMockeyDrainReaderChannel_Empty(t *testing.T) {
	mockey.PatchConvey("drainReaderChannel does nothing on empty channel", t, func() {
		s := &Session{
			reader: make(chan Body, 10),
		}
		// Should not panic or block
		s.drainReaderChannel()
	})
}

func TestMockeyDrainReaderChannel_WithMessages(t *testing.T) {
	mockey.PatchConvey("drainReaderChannel removes all messages", t, func() {
		s := &Session{
			reader: make(chan Body, 10),
		}
		s.reader <- Body{ReqID: "stale-1"}
		s.reader <- Body{ReqID: "stale-2"}
		s.reader <- Body{ReqID: "stale-3"}

		s.drainReaderChannel()

		// Channel should be empty now
		select {
		case <-s.reader:
			assert.Fail(t, "channel should be empty after drain")
		default:
			// expected
		}
	})
}

// --- createSessionRequest tests ---

func TestMockeyCreateSessionRequest_ValidRequest(t *testing.T) {
	mockey.PatchConvey("createSessionRequest creates request with correct headers", t, func() {
		ctx := context.Background()
		req, err := createSessionRequest(ctx, "https://control.example.com", "machine-1", "read", "bearer-token", nil)

		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Equal(t, "machine-1", req.Header.Get("X-GPUD-Machine-ID"))
		assert.Equal(t, "read", req.Header.Get("X-GPUD-Session-Type"))
		assert.Equal(t, "Bearer bearer-token", req.Header.Get("Authorization"))

		// Check deprecated headers
		assert.Equal(t, "machine-1", req.Header.Get("machine_id"))
		assert.Equal(t, "read", req.Header.Get("session_type"))
		assert.Equal(t, "bearer-token", req.Header.Get("token"))
	})
}

func TestMockeyCreateSessionRequest_InvalidURL(t *testing.T) {
	mockey.PatchConvey("createSessionRequest returns error for invalid URL", t, func() {
		ctx := context.Background()
		_, err := createSessionRequest(ctx, "://invalid", "machine-1", "read", "token", nil)
		require.Error(t, err)
	})
}

func TestMockeyCreateSessionRequest_EmptyHost(t *testing.T) {
	mockey.PatchConvey("createSessionRequest returns error for empty host", t, func() {
		ctx := context.Background()
		_, err := createSessionRequest(ctx, "https://", "machine-1", "read", "token", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no host")
	})
}

// --- createHTTPClient tests ---

func TestMockeyCreateHTTPClient(t *testing.T) {
	mockey.PatchConvey("createHTTPClient creates client with correct settings", t, func() {
		jar, _ := cookiejar.New(nil)
		client := createHTTPClient(jar)

		require.NotNil(t, client)
		assert.Equal(t, jar, client.Jar)

		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok)
		assert.True(t, transport.DisableKeepAlives)
		assert.Equal(t, 10, transport.MaxIdleConns)
	})
}

// --- validateTokenWithHealthCheck tests ---

func TestMockeyValidateToken_Success(t *testing.T) {
	mockey.PatchConvey("validateTokenWithHealthCheck succeeds when health check passes", t, func() {
		s := &Session{
			checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return nil
			},
		}

		err := s.validateTokenWithHealthCheck(context.Background(), "valid-token")
		assert.NoError(t, err)
	})
}

func TestMockeyValidateToken_Failure(t *testing.T) {
	mockey.PatchConvey("validateTokenWithHealthCheck returns error when health check fails", t, func() {
		s := &Session{
			checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return errors.New("unauthorized")
			},
		}

		err := s.validateTokenWithHealthCheck(context.Background(), "bad-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "health check with new token failed")
		assert.Contains(t, err.Error(), "unauthorized")
	})
}

// --- checkServerHealth tests ---

func TestMockeyCheckServerHealth_GatewayHost(t *testing.T) {
	mockey.PatchConvey("checkServerHealth skips health check for gpud-gateway hosts", t, func() {
		s := &Session{
			epControlPlane: "https://gpud-gateway.example.com",
		}

		jar, _ := cookiejar.New(nil)
		err := s.checkServerHealth(context.Background(), jar, "token")

		assert.NoError(t, err)
	})
}

func TestMockeyCheckServerHealth_Success(t *testing.T) {
	mockey.PatchConvey("checkServerHealth succeeds with 200 OK response", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s := &Session{
			epControlPlane: server.URL,
		}

		jar, _ := cookiejar.New(nil)
		err := s.checkServerHealth(context.Background(), jar, "test-token")

		assert.NoError(t, err)
	})
}

func TestMockeyCheckServerHealth_ServerError(t *testing.T) {
	mockey.PatchConvey("checkServerHealth returns error on non-200 response", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		s := &Session{
			epControlPlane: server.URL,
		}

		jar, _ := cookiejar.New(nil)
		err := s.checkServerHealth(context.Background(), jar, "test-token")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "server health check failed")
	})
}

func TestMockeyCheckServerHealth_UsesSessionTokenWhenEmpty(t *testing.T) {
	mockey.PatchConvey("checkServerHealth uses session token when no token provided", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer session-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s := &Session{
			epControlPlane: server.URL,
			token:          "session-token",
		}

		jar, _ := cookiejar.New(nil)
		err := s.checkServerHealth(context.Background(), jar, "")

		assert.NoError(t, err)
	})
}

// --- DefaultQuerySince and initializeGracePeriod constants ---

func TestMockeyConstants(t *testing.T) {
	mockey.PatchConvey("DefaultQuerySince and initializeGracePeriod have expected values", t, func() {
		assert.Equal(t, 30*time.Minute, DefaultQuerySince)
		assert.Equal(t, 5*time.Minute, initializeGracePeriod)
	})
}

// --- Op.WithDB tests ---

func TestMockeyOpOption_WithDB(t *testing.T) {
	mockey.PatchConvey("WithDB sets both dbRW and dbRO on Op", t, func() {
		op := &Op{}
		dbRW := &sql.DB{}
		dbRO := &sql.DB{}
		opt := WithDB(dbRW, dbRO)
		opt(op)
		assert.Equal(t, dbRW, op.dbRW)
		assert.Equal(t, dbRO, op.dbRO)
	})
}

// --- Op.WithMetricsStore tests ---

func TestMockeyOpOption_WithMetricsStore(t *testing.T) {
	mockey.PatchConvey("WithMetricsStore sets metrics store on Op", t, func() {
		op := &Op{}
		store := new(mockMetricsStore)
		opt := WithMetricsStore(store)
		opt(op)
		assert.Equal(t, store, op.metricsStore)
	})
}

// --- Op.WithSavePluginSpecsFunc tests ---

func TestMockeyOpOption_WithSavePluginSpecsFunc(t *testing.T) {
	mockey.PatchConvey("WithSavePluginSpecsFunc sets save func on Op", t, func() {
		op := &Op{}
		saveFunc := func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
			return true, nil
		}
		opt := WithSavePluginSpecsFunc(saveFunc)
		opt(op)
		assert.NotNil(t, op.savePluginSpecsFunc)
	})
}

// --- Op.WithComponentsRegistry tests ---

func TestMockeyOpOption_WithComponentsRegistry(t *testing.T) {
	mockey.PatchConvey("WithComponentsRegistry sets registry on Op", t, func() {
		op := &Op{}
		registry := new(mockComponentRegistry)
		opt := WithComponentsRegistry(registry)
		opt(op)
		assert.Equal(t, registry, op.componentsRegistry)
	})
}

// --- ErrAutoUpdateDisabledButExitCodeSet constant test ---

func TestMockeyErrAutoUpdateDisabledButExitCodeSet(t *testing.T) {
	mockey.PatchConvey("ErrAutoUpdateDisabledButExitCodeSet has expected message", t, func() {
		assert.NotNil(t, ErrAutoUpdateDisabledButExitCodeSet)
		assert.Contains(t, ErrAutoUpdateDisabledButExitCodeSet.Error(), "auto update is disabled")
	})
}

func TestMockeyProcessRequestAsync_UnsupportedMethod(t *testing.T) {
	mockey.PatchConvey("processRequestAsync sets error for unsupported methods", t, func() {
		s := &Session{
			ctx: context.Background(),
		}

		var gotMethod string
		var gotResp *Response
		mockey.Mock((*Session).sendResponse).To(func(_ *Session, reqID, method string, response *Response) {
			gotMethod = method
			gotResp = response
		}).Build()

		s.processRequestAsync("req-123", "unknownMethod", Request{})

		assert.Equal(t, "unknownMethod", gotMethod)
		require.NotNil(t, gotResp)
		assert.Equal(t, int32(http.StatusBadRequest), gotResp.ErrorCode)
		assert.Contains(t, gotResp.Error, "unsupported async method")
	})
}

func TestMockeySendResponse_MarshalError(t *testing.T) {
	mockey.PatchConvey("sendResponse returns early when marshal fails", t, func() {
		s := &Session{
			ctx:            context.Background(),
			auditLogger:    log.NewNopAuditLogger(),
			epControlPlane: "https://example.com",
			machineID:      "machine-1",
			writer:         make(chan Body, 1),
		}

		resp := &Response{
			Metrics: apiv1.GPUdComponentMetrics{
				{
					Component: "test-component",
					Metrics: apiv1.Metrics{
						{
							UnixSeconds: 1,
							Name:        "bad-metric",
							Value:       math.NaN(),
						},
					},
				},
			},
		}

		s.sendResponse("req-456", "metrics", resp)

		select {
		case <-s.writer:
			assert.Fail(t, "expected no response when marshal fails")
		default:
		}
	})
}

func TestMockeyProcessRequest_UpdateConfigSkipped_NoProcess(t *testing.T) {
	mockey.PatchConvey("processRequest skips updateConfig when disabled", t, func() {
		s := &Session{
			ctx:              context.Background(),
			skipUpdateConfig: true,
		}
		response := &Response{}
		restartExitCode := -1

		called := false
		mockey.Mock((*Session).processUpdateConfig).To(func(_ *Session, configMap map[string]string, resp *Response) {
			called = true
		}).Build()

		payload := Request{
			Method: "updateConfig",
			UpdateConfig: map[string]string{
				"fake-component": "{}",
			},
		}

		handledAsync := s.processRequest(context.Background(), "req-789", payload, response, &restartExitCode)

		assert.False(t, handledAsync)
		assert.False(t, called, "processUpdateConfig should not be invoked when skipUpdateConfig is true")
	})
}
