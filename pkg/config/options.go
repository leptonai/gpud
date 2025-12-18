package config

import (
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
)

type Op struct {
	pkgconfigcommon.ToolOverwrites

	FailureInjector *components.FailureInjector

	DataDir    string
	DBInMemory bool

	// SessionToken is the session token for db-in-memory mode.
	// When DBInMemory is true and this is set, the server will seed
	// this token into the in-memory database.
	SessionToken string

	// SessionMachineID is the machine ID for db-in-memory mode.
	// When DBInMemory is true and this is set, the server will seed
	// this machine ID into the in-memory database.
	SessionMachineID string

	// SessionEndpoint is the control plane endpoint for db-in-memory mode.
	// When DBInMemory is true and this is set, the server will seed
	// this endpoint into the in-memory database.
	// The server reads the endpoint from metadata DB, so it must be seeded for in-memory mode.
	SessionEndpoint string
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.InfinibandClassRootDir == "" {
		op.InfinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// Specifies the root directory of the InfiniBand class.
func WithInfinibandClassRootDir(p string) OpOption {
	return func(op *Op) {
		op.InfinibandClassRootDir = p
	}
}

// WithExcludedInfinibandDevices sets the list of InfiniBand device names to exclude from monitoring.
// Device names should be like "mlx5_0", "mlx5_1", etc. (not full paths).
//
// This is useful for excluding devices that have restricted Physical Functions (PFs)
// and cause kernel errors (mlx5_cmd_out_err ACCESS_REG) when queried.
// This is common on NVIDIA DGX, Umbriel, and GB200 systems with ConnectX-7 adapters.
//
// ref.
// https://github.com/prometheus/node_exporter/issues/3434
// https://github.com/leptonai/gpud/issues/1164
func WithExcludedInfinibandDevices(devices []string) OpOption {
	return func(op *Op) {
		op.ExcludedInfinibandDevices = devices
	}
}

func WithFailureInjector(injector *components.FailureInjector) OpOption {
	return func(op *Op) {
		op.FailureInjector = injector
	}
}

// WithDataDir overrides the default data directory for GPUd artifacts.
func WithDataDir(dataDir string) OpOption {
	return func(op *Op) {
		op.DataDir = dataDir
	}
}

// WithDBInMemory enables in-memory SQLite database mode.
// When true, uses file::memory:?cache=shared instead of file-based storage.
// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
func WithDBInMemory(b bool) OpOption {
	return func(op *Op) {
		op.DBInMemory = b
	}
}

// WithSessionToken sets the session token for db-in-memory mode.
// When DBInMemory is true and this is set, the server will seed
// this token into the in-memory database for session authentication.
func WithSessionToken(token string) OpOption {
	return func(op *Op) {
		op.SessionToken = token
	}
}

// WithSessionMachineID sets the machine ID for db-in-memory mode.
// When DBInMemory is true and this is set, the server will seed
// this machine ID into the in-memory database.
func WithSessionMachineID(machineID string) OpOption {
	return func(op *Op) {
		op.SessionMachineID = machineID
	}
}

// WithSessionEndpoint sets the control plane endpoint for db-in-memory mode.
// When DBInMemory is true and this is set, the server will seed
// this endpoint into the in-memory database.
// The server reads the endpoint from metadata DB, so it must be seeded for in-memory mode.
func WithSessionEndpoint(endpoint string) OpOption {
	return func(op *Op) {
		op.SessionEndpoint = endpoint
	}
}
