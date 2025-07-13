package components

import (
	"context"
	"database/sql"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// GPUdInstance is the instance of the GPUd dependencies.
type GPUdInstance struct {
	RootCtx context.Context

	// MachineID is either the machine ID assigned from the control plane
	// or the unique UUID of the machine.
	// For example, it is used to identify itself for the NFS checker.
	MachineID string

	KernelModulesToCheck []string

	NVMLInstance         nvidianvml.Instance
	NVIDIAToolOverwrites nvidiacommon.ToolOverwrites

	DBRW *sql.DB
	DBRO *sql.DB

	SuggestedActionsStore SuggestedActionsStore
	EventStore            eventstore.Store
	RebootEventStore      pkghost.RebootEventStore

	MountPoints  []string
	MountTargets []string
}
