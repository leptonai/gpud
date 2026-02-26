package login

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/httputil"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/server"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

// TestLogin_EmptyToken tests that Login returns error when token is empty.
func TestLogin_EmptyToken(t *testing.T) {
	ctx := context.Background()
	cfg := LoginConfig{
		Token:    "",
		Endpoint: "https://example.com",
		DataDir:  "/tmp/test",
	}

	err := Login(ctx, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyToken)
}

// TestLogin_ResolveDataDirError tests error handling when data dir resolution fails.
func TestLogin_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("resolve data dir error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/invalid/path",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestLogin_OpenDatabaseError tests error handling when database open fails.
func TestLogin_OpenDatabaseError(t *testing.T) {
	mockey.PatchConvey("open database error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestLogin_CreateMetadataTableError tests error handling when metadata table creation fails.
func TestLogin_CreateMetadataTableError(t *testing.T) {
	mockey.PatchConvey("create metadata table error", t, func() {
		mockDB := &sql.DB{}

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return errors.New("failed to create table")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create metadata table")
	})
}

// TestLogin_CreateSessionStatesTableError tests error handling when session states table creation fails.
func TestLogin_CreateSessionStatesTableError(t *testing.T) {
	mockey.PatchConvey("create session states table error", t, func() {
		mockDB := &sql.DB{}

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return errors.New("failed to create session states table")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create session states table")
	})
}

// TestLogin_ReadMachineIDError tests error handling when reading machine ID fails.
func TestLogin_ReadMachineIDError(t *testing.T) {
	mockey.PatchConvey("read machine id error", t, func() {
		mockDB := &sql.DB{}

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read machine id")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine id")
	})
}

// TestLogin_MachineIDAlreadyAssigned tests that Login skips when machine ID is already assigned.
func TestLogin_MachineIDAlreadyAssigned(t *testing.T) {
	mockey.PatchConvey("machine id already assigned", t, func() {
		mockDB := &sql.DB{}

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "existing-machine-id", nil
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
	})
}

// TestLogin_CreateNVMLInstanceError tests error handling when NVML instance creation fails.
func TestLogin_CreateNVMLInstanceError(t *testing.T) {
	mockey.PatchConvey("create nvml instance error", t, func() {
		mockDB := &sql.DB{}

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return nil, errors.New("nvml not found")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create nvml instance")
	})
}

// TestLogin_CreateLoginRequestError tests error handling when creating login request fails.
func TestLogin_CreateLoginRequestError(t *testing.T) {
	mockey.PatchConvey("create login request error", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return nil, errors.New("failed to get machine info")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create login request")
	})
}

// TestLogin_SendRequestError tests error handling when sending request fails.
func TestLogin_SendRequestError(t *testing.T) {
	mockey.PatchConvey("send request error", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{
				Network: &apiv1.MachineNetwork{
					PublicIP:  "1.2.3.4",
					PrivateIP: "10.0.0.1",
				},
			}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return nil, errors.New("network error")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})
}

// TestLogin_SendRequestErrorWithResponse tests error handling when send request returns error with response.
func TestLogin_SendRequestErrorWithResponse(t *testing.T) {
	mockey.PatchConvey("send request error with response", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				Code:    "401",
				Message: "invalid token",
			}, errors.New("authentication failed")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to login")
		assert.Contains(t, err.Error(), "401")
		assert.Contains(t, err.Error(), "invalid token")
	})
}

// TestLogin_SetMetadataEndpointError tests error handling when setting endpoint fails.
func TestLogin_SetMetadataEndpointError(t *testing.T) {
	mockey.PatchConvey("set metadata endpoint error", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		// First SetMetadata call (endpoint) fails
		callCount := 0
		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			callCount++
			if callCount == 1 {
				return errors.New("failed to set endpoint")
			}
			return nil
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record endpoint")
	})
}

// TestLogin_Success tests successful login flow.
func TestLogin_Success(t *testing.T) {
	mockey.PatchConvey("successful login", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{
				Network: &apiv1.MachineNetwork{
					PublicIP:  "1.2.3.4",
					PrivateIP: "10.0.0.1",
				},
			}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
	})
}

// TestLogin_SuccessWithPublicPrivateIPOverride tests successful login with IP overrides.
func TestLogin_SuccessWithPublicPrivateIPOverride(t *testing.T) {
	mockey.PatchConvey("successful login with IP overrides", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		receivedRequest := &apiv1.LoginRequest{}
		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{
				Network: &apiv1.MachineNetwork{
					PublicIP:  "1.2.3.4",
					PrivateIP: "10.0.0.1",
				},
			}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			*receivedRequest = req
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:     "test-token",
			Endpoint:  "https://example.com",
			DataDir:   "/tmp/test",
			PublicIP:  "5.6.7.8",
			PrivateIP: "192.168.1.1",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)

		// Verify IP overrides were applied
		assert.Equal(t, "5.6.7.8", receivedRequest.Network.PublicIP)
		assert.Equal(t, "192.168.1.1", receivedRequest.Network.PrivateIP)
	})
}

// TestLogin_SuccessWithServerRunning tests successful login when server is already running.
func TestLogin_SuccessWithServerRunning(t *testing.T) {
	mockey.PatchConvey("successful login with server running", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return true
		}).Build()

		writeTokenCalled := false
		mockey.Mock(server.WriteToken).To(func(token, fifoFile string) error {
			writeTokenCalled = true
			return nil
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
		assert.True(t, writeTokenCalled, "expected WriteToken to be called")
	})
}

// TestLogin_SuccessWithValidationResults tests successful login with validation results.
func TestLogin_SuccessWithValidationResults(t *testing.T) {
	mockey.PatchConvey("successful login with validation results", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
				ValidationResults: []apiv1.ValidationResult{
					{Name: "check1", Valid: true},
					{Name: "check2", Valid: false, Reason: "failed", Suggestion: "fix it"},
				},
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
	})
}

// TestServerRunning_NoSystemctl tests serverRunning when systemctl doesn't exist.
func TestServerRunning_NoSystemctl(t *testing.T) {
	mockey.PatchConvey("no systemctl", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		result := serverRunning()
		assert.False(t, result)
	})
}

// TestServerRunning_SystemctlExistsInactive tests serverRunning when service is inactive.
func TestServerRunning_SystemctlExistsInactive(t *testing.T) {
	mockey.PatchConvey("systemctl exists but service inactive", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		result := serverRunning()
		assert.False(t, result)
	})
}

// TestServerRunning_SystemctlExistsActive tests serverRunning when service is active.
func TestServerRunning_SystemctlExistsActive(t *testing.T) {
	mockey.PatchConvey("systemctl exists and service active", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return true, nil
		}).Build()

		result := serverRunning()
		assert.True(t, result)
	})
}

// TestServerRunning_SystemctlError tests serverRunning when systemctl check fails.
func TestServerRunning_SystemctlError(t *testing.T) {
	mockey.PatchConvey("systemctl check error", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, errors.New("systemctl failed")
		}).Build()

		result := serverRunning()
		assert.False(t, result)
	})
}

// TestErrEmptyToken tests that the error constant exists and has a meaningful message.
func TestErrEmptyToken(t *testing.T) {
	assert.NotNil(t, ErrEmptyToken)
	assert.Contains(t, ErrEmptyToken.Error(), "token")
}

// TestLoginConfig_ZeroValue tests the zero value of LoginConfig.
func TestLoginConfig_ZeroValue(t *testing.T) {
	var cfg LoginConfig
	assert.Empty(t, cfg.Token)
	assert.Empty(t, cfg.Endpoint)
	assert.Empty(t, cfg.MachineID)
	assert.Empty(t, cfg.NodeGroup)
	assert.Empty(t, cfg.DataDir)
	assert.Empty(t, cfg.GPUCount)
	assert.Empty(t, cfg.PublicIP)
	assert.Empty(t, cfg.PrivateIP)
}

// TestLogin_WithMachineIDAndNodeGroup tests login with both machine ID and node group specified.
func TestLogin_WithMachineIDAndNodeGroup(t *testing.T) {
	mockey.PatchConvey("login with machine ID and node group", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		var receivedMachineID, receivedNodeGroup string
		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			receivedMachineID = machineID
			receivedNodeGroup = nodeGroup
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:     "test-token",
			Endpoint:  "https://example.com",
			DataDir:   "/tmp/test",
			MachineID: "my-machine-id",
			NodeGroup: "my-node-group",
			GPUCount:  "8",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)

		assert.Equal(t, "my-machine-id", receivedMachineID)
		assert.Equal(t, "my-node-group", receivedNodeGroup)
	})
}

// TestLogin_NVMLShutdownError tests that NVML shutdown errors don't fail the login.
func TestLogin_NVMLShutdownError(t *testing.T) {
	mockey.PatchConvey("nvml shutdown error doesn't fail login", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		// NVML shutdown fails but shouldn't prevent successful login
		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return errors.New("shutdown failed")
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
	})
}

// TestLogin_WithGPUCount tests login with GPU count specified.
func TestLogin_WithGPUCount(t *testing.T) {
	mockey.PatchConvey("login with GPU count", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		var receivedGPUCount string
		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			receivedGPUCount = gpuCount
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				MachineID: "machine-123",
				Token:     "session-token",
			}, nil
		}).Build()

		mockey.Mock(pkgmetadata.SetMetadata).To(func(ctx context.Context, db *sql.DB, key, value string) error {
			return nil
		}).Build()

		mockey.Mock(config.FifoFilePath).To(func(dataDir string) string {
			return "/tmp/test/fifo"
		}).Build()

		mockey.Mock(serverRunning).To(func() bool {
			return false
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
			GPUCount: "16",
		}

		err := Login(ctx, cfg)
		require.NoError(t, err)
		assert.Equal(t, "16", receivedGPUCount)
	})
}

// TestLogin_SendRequestWithDeprecatedErrorField tests error handling with deprecated Error field.
func TestLogin_SendRequestWithDeprecatedErrorField(t *testing.T) {
	mockey.PatchConvey("send request with deprecated error field", t, func() {
		mockDB := &sql.DB{}
		mockNVML := nvidianvml.NewNoOp()

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return mockDB, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.CreateTableMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock((nvidianvml.Instance).Shutdown).To(func() error {
			return nil
		}).Build()

		mockey.Mock(pkgmachineinfo.CreateLoginRequest).To(func(token, machineID, nodeGroup, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
			return &apiv1.LoginRequest{Network: &apiv1.MachineNetwork{}}, nil
		}).Build()

		mockey.Mock(SendRequest).To(func(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return &apiv1.LoginResponse{
				Status: "400",                  // deprecated field
				Error:  "invalid request body", // deprecated field
			}, errors.New("bad request")
		}).Build()

		ctx := context.Background()
		cfg := LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to login")
		assert.Contains(t, err.Error(), "400")
		assert.Contains(t, err.Error(), "invalid request body")
	})
}

// TestSendRequest_URLCreationError tests error handling when URL creation fails.
func TestSendRequestWrapper_URLCreationError(t *testing.T) {
	mockey.PatchConvey("URL creation error", t, func() {
		mockey.Mock(httputil.CreateURL).To(func(scheme, host, path string) (string, error) {
			return "", errors.New("invalid URL format")
		}).Build()

		ctx := context.Background()
		req := apiv1.LoginRequest{}

		resp, err := SendRequest(ctx, "invalid-endpoint", req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error creating URL")
		assert.Nil(t, resp)
	})
}

// TestSendRequest_Success tests successful URL creation and delegation to sendRequest.
func TestSendRequestWrapper_Success(t *testing.T) {
	mockey.PatchConvey("successful send request", t, func() {
		expectedURL := "https://example.com/api/v1/login"
		mockey.Mock(httputil.CreateURL).To(func(scheme, host, path string) (string, error) {
			return expectedURL, nil
		}).Build()

		var receivedURL string
		mockey.Mock(sendRequest).To(func(ctx context.Context, endpointURL string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			receivedURL = endpointURL
			return &apiv1.LoginResponse{
				MachineID: "test-machine-id",
				Token:     "test-token",
			}, nil
		}).Build()

		ctx := context.Background()
		req := apiv1.LoginRequest{}

		resp, err := SendRequest(ctx, "example.com", req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-machine-id", resp.MachineID)
		assert.Equal(t, expectedURL, receivedURL)
	})
}

// TestSendRequest_SendRequestError tests error propagation from sendRequest.
func TestSendRequestWrapper_SendRequestError(t *testing.T) {
	mockey.PatchConvey("sendRequest returns error", t, func() {
		mockey.Mock(httputil.CreateURL).To(func(scheme, host, path string) (string, error) {
			return "https://example.com/api/v1/login", nil
		}).Build()

		mockey.Mock(sendRequest).To(func(ctx context.Context, endpointURL string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
			return nil, errors.New("network timeout")
		}).Build()

		ctx := context.Background()
		req := apiv1.LoginRequest{}

		resp, err := SendRequest(ctx, "example.com", req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network timeout")
		assert.Nil(t, resp)
	})
}
